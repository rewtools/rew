# REW Architecture

## Status

This document describes the experimental schema-v3 implementation.

It is not a specification for future releases. The archive format and implementation details may change.

## Purpose

REW attempts to reproduce certain failed Go builds using a captured subset of the original inputs.

Its architecture consists of four main stages:

1. Capture
2. Capsule verification
3. Workspace and dependency staging
4. Offline Docker replay and failure comparison

## Repository Structure

| Directory | Responsibility |
|---|---|
| `cmd/rew` | CLI argument handling and capture entry points |
| `internal/capsule` | Manifest, archive writing, verification, and restoration |
| `internal/execution` | Captured command execution |
| `internal/replay` | Replay orchestration, portability checks, and result comparison |
| `internal/runtime/docker` | Docker execution, restrictions, and lifecycle management |
| `testdata` | Integration test fixtures |

## Supported v3 Execution Flow

```text
Original Go workspace
        |
        v
rew capture --v3
        |
        +-- Validate supported command and runtime
        |
        +-- Inspect Go build environment
        |
        +-- Read selected workspace files
        |
        +-- Execute original Go build
        |
        +-- On failure, verify workspace consistency
        |
        +-- Resolve selected Go module graph offline
        |
        +-- Read dependency artifacts from Go module cache
        |
        +-- Verify module content and checksums
        |
        v
    failure.rew
        |
        v
rew replay --v3 --allow-command
        |
        +-- Verify capsule contents
        |
        +-- Check platform and runtime compatibility
        |
        +-- Construct verified workspace and module-cache TAR snapshots
        |
        +-- Upload snapshots into Docker-managed volumes
        |
        +-- Mount volumes read-only in the execution container
        |
        v
Restricted Docker container
        |
        +-- Prepare offline module cache
        |
        +-- Verify exact selected Go module graph
        |
        +-- Execute recorded Go build
        |
        +-- Capture bounded output and exit status
        |
        v
Compare original and replayed failures
        |
        v
Outcome
```

## Capture

The schema-v3 entry point is implemented in:

`cmd/rew/capture_v3.go`

The CLI currently accepts only the supported command:

`go build ./...`

The capture stage inspects the Go environment before executing the original build.

If the build succeeds, no failure capsule is written.

If it fails, REW requires complete, nonempty stderr and attempts to package the recorded failure.

The user must explicitly select workspace files using `--include`.

Schema v3 accepts at most 256 included regular workspace files, while schema v2 retains its existing 64-file limit. The combined workspace data limit remains 14 MiB, with a maximum of 10 MiB per file. These limits are enforced during capture, manifest validation, verified reading, and staging verification.

For the current supported dependency scenario, both `go.mod` and `go.sum` must be included.

## Go Dependency Capture

The schema-v3 implementation in v0.1.0-alpha.3 supports up to 128
selected external Go modules, including transitive dependencies. At most
64 modules may require complete ZIP artifacts.

Capture first resolves the complete selected module graph offline using
`go list -m -json all`. It also inspects `go list -deps -e -json ./...`
to identify modules needed by build packages.

For build-required modules, Capture reads complete cache artifacts:

- `.zip`
- `.mod`
- `.info`

Other selected modules retain graph metadata without full ZIP content.
The module path and version determine the expected artifact locations.

Capture also runs `go mod graph` with Go module proxy access disabled.
This identifies older or intermediate module versions whose `.mod`
metadata may be required for offline graph reconstruction even though
those versions are not selected for the build.

These graph-only `.mod` files are stored separately from the selected
modules' `.zip`, `.mod`, and `.info` artifacts. Capture permits at most
128 graph-only metadata files with a combined size of 1 MiB. Each file
is verified against SHA-256 and its corresponding `go.sum` `/go.mod`
checksum before packaging.

Replay reconstructs both sets inside the isolated container before
checking the module graph and executing the recorded build.

Captured artifacts are checked against the supported module format and the checksums recorded in `go.sum`.

Module graph resolution disables proxy access and uses the local
module cache. Missing artifacts and unsupported module configurations
are rejected rather than silently downloaded or captured.

The selected graph is checked again inside the Docker replay container
before the recorded build command is executed.

## Disk-Backed Artifact Processing

Schema-v3 capture and replay use bounded, disk-backed processing for
larger Go projects rather than retaining all dependency artifacts and
prepared snapshots in memory.

During replay, REW verifies capsule contents into a temporary
artifact store and constructs TAR snapshots from verified readers.
Snapshot files are passed to the Docker upload path using retained file
descriptors, avoiding pathname-based reopening after preparation.

The snapshots are uploaded into Docker-managed volumes, which are
mounted read-only in the execution container. The replay container
runs without network access.

The schema-v3 capsule limit is 96 MiB. Complete Go module artifacts
have a combined 64 MiB budget. Verified ZIP contents are limited to
48 MiB unpacked per module and 256 MiB unpacked across all selected
complete Go modules.

These mechanisms reduce resource usage and mitigate specific filesystem
race risks, but they do not make arbitrary untrusted capsules safe to
execute. See [Replay Staging Security Model](security-model.md).

## Capsule Format

A schema-v3 capsule is an archive containing a manifest, selected workspace content, and dependency artifacts. New captures use `.rew`; existing `.bfc` capsules remain readable. The filename extension does not change the internal schema.

Schema-v3 records information including:

- Schema version
- Supported runtime
- Source platform
- Go build environment
- Original command arguments
- Exit status
- Captured stdout and stderr
- Workspace integrity metadata
- Dependency identity and artifact metadata

The current format is experimental.

Do not assume compatibility with future schema versions.

## Verification

Capsule verification is handled by code under `internal/capsule`.

Validation includes structural checks, supported metadata, workspace integrity, and module artifact verification.

SHA-256 verifies recorded file content against the manifest.

Go module checksum validation provides an additional consistency check for dependency contents.

These checks detect classes of malformed or modified inputs.

They do not authenticate the capsule creator and do not establish that captured source code is safe.

## Replay Preparation

The replay orchestrator is implemented in:

`internal/replay/replay_v3.go`

Before Docker execution, the replay path:

1. Verifies the capsule.
2. Evaluates runtime and platform compatibility.
3. Requires explicit command-execution authorization.
4. Builds workspace and module-cache TAR snapshots from verified data.
5. Uploads snapshots into Docker-managed volumes.
6. Mounts both volumes read-only in the execution container.
7. Prepares the offline module cache and verifies the exact module graph
   before executing the captured command.

The replay implementation does not reconstruct an entire original machine, operating system, or CI runner.

## Docker Execution

The Docker integration lives under:

`internal/runtime/docker`

Schema-v3 replay uses a pinned Go runtime image and a constrained container configuration.

The implemented controls include:

- Disabled container networking
- Read-only container root filesystem
- Non-root process execution
- Dropped Linux capabilities
- Restricted privilege escalation
- Resource limits
- Bounded output capture
- Container cleanup

The module artifacts are made available to the container, where the Go module cache is prepared before the recorded build is executed.

This pre-execution preparation is part of replay orchestration. It is not an exact reproduction of every operation originally performed on the capture host.

Docker is not a complete security boundary for malicious workloads.

## Failure Comparison

REW compares recorded and replayed failure results.

For the supported schema-v3 scenario, successful reproduction requires matching failure characteristics, including the exit status and stderr.

A `reproduced` outcome means the implemented comparison accepted the replay result.

It does not prove complete environmental equivalence.

## Portability

The verified cross-platform CI scenario is:

- Capture host: macOS ARM64
- Go build target: Linux AMD64
- CGO: disabled
- Replay host: Linux AMD64
- Replay runtime in the verified cross-platform CI scenario: pinned Go 1.26.6 Docker environment
- Additional schema-v3 Go 1.25.14 runtime: locally verified with macOS ARM64 capture and Linux AMD64 Docker replay; independent CI validation pending

The capsule is transferred between independent GitHub Actions runners.

This validates that specific supported scenario, not arbitrary cross-platform builds.

## Security Boundaries and Known Gaps

The most important trust boundaries are:

- Capture workspace to archive
- Archive to replay staging
- Staging to Docker container
- Container runtime to host kernel

Important known gaps include:

- No authenticated capsule provenance
- No security guarantee for unknown or hostile capsules
- Incomplete protection against concurrent staging changes
- Limited Go dependency graph support
- Limited command and platform support
- No complete CI environment snapshot

See `SECURITY.md` for the security policy and threat model.

## Test Coverage

The test suite contains:

- Unit tests for archive and manifest validation
- Go module checksum and artifact tests
- Workspace restoration and staging integrity tests
- Capture CLI tests
- Docker execution and cancellation tests
- End-to-end replay tests
- Cross-platform GitHub Actions jobs

Run the ordinary suite with:

```sh
go test -count=1 ./...
```

Docker integration tests require an available Docker daemon and the relevant test environment configuration.

## Design Principle

REW currently prioritizes a small, explicit, verifiable execution model over broad but weakly specified CI compatibility.

New runtime, dependency, or platform support should come with corresponding verification rules and end-to-end tests.
