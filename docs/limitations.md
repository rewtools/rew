# REW — Supported Scope and Limitations

## Status

This document describes the schema-v3 implementation planned for v0.1.0-alpha.6.
Earlier alpha releases may have different limits and capabilities.

REW is not production-ready. Supported scenarios are intentionally narrow, and successful reproduction is not guaranteed outside the tested configurations.

## Support Matrix

| Component | Current support |
|---|---|
| Capture language | Go |
| Go toolchain | Schema v3: Go 1.25.14 or Go 1.26.6 (exact patch versions) |
| Captured command | `go build ./...` |
| External modules | Up to 128 selected external modules; up to 64 full ZIP modules; offline graph resolution required |
| Dependency artifacts | `.zip`, `.mod`, `.info` |
| Dependency verification | Go module h1 checksums and artifact integrity |
| Workspace capture | Explicitly included regular files |
| Capsule format | Experimental schema-v3; new filename extension `.rew`, legacy `.bfc` accepted |
| Replay target | Linux AMD64 |
| Replay runtime | Version-specific pinned Go Docker image matching the capsule runtime |
| Container networking | Disabled |
| Replay authorization | Explicit `--allow-command` |
| Failure comparison | Exit code and stderr |
| Capsule provenance authentication | Not supported |
| Untrusted capsule execution | Not supported as a safe use case |

## Size and Module Limits

The schema-v3 implementation enforces:

- Maximum 128 selected external Go modules.
- Maximum 64 modules with full ZIP artifacts.
- Maximum 128 additional graph-only `.mod` files, 1 MiB combined.
- Maximum 64 MiB combined module artifact data.
- Maximum 48 MiB unpacked ZIP contents per complete Go module.
- Maximum 256 MiB combined unpacked ZIP contents across selected complete Go modules.
- Maximum 96 MiB total schema-v3 capsule archive size (both `.rew` and `.bfc`).
- Maximum 2048 explicitly included workspace files for schema v3.
- Schema-v3 workspace paths must be representable in USTAR TAR headers without requiring PAX path extensions.
- Schema v2 retains its 64-file workspace limit.
- Maximum 10 MiB per workspace file.
- Maximum 14 MiB combined workspace file data.

Build-required external modules need complete `.zip`, `.mod`, and `.info`
artifacts in the local Go module download cache. Other selected modules
require metadata sufficient for offline graph verification.

Additional module versions referenced by `go mod graph`, but not selected
for the build, require their cached `.mod` metadata. These graph-only
artifacts must match the `/go.mod` checksums recorded in `go.sum`.
Capture rejects missing or invalid metadata rather than downloading it.

The complete selected module graph must match the verified manifest
inside the offline Docker container before the recorded command runs.

## Capture Consistency

The original Go build runs on the capture host, not in the restricted
replay container.

Capture reads explicitly included workspace files before execution
and checks their contents again after execution and before packaging.

These checks are not an atomic filesystem snapshot. Concurrent changes
that are reversed between checks may escape detection.

## Verified Cross-Platform Scenario

GitHub Actions currently validates:

1. Capture execution on a macOS ARM64 runner.
2. Compilation targeting Linux AMD64.
3. `CGO_ENABLED=0`.
4. One external Go module dependency.
5. Capsule transfer through a GitHub Actions artifact.
6. Replay inside a Linux AMD64 Docker environment.
7. A `reproduced` outcome.

This is a specific cross-host reproduction scenario.

It does not establish universal cross-platform reproducibility.

## Command Restrictions

Schema-v3 currently accepts only:

```sh
go build ./...
```

Other commands are outside the supported scope, including:

- `go test ./...`
- `go run .`
- `go generate`
- `make`
- Shell scripts and shell wrappers
- Arbitrary compiler invocations
- Go build commands with additional flags

These restrictions allow the implementation to make more explicit assumptions about the execution environment.

## Schema v3 Reader Compatibility

Schema v3 supports up to 64 complete module ZIP snapshots and a
96 MiB capsule size limit. Capsules requiring graph-only module metadata add the optional
`go_graph_mod_files` manifest field and corresponding verified artifacts.

REW v0.1.0-alpha.2 readers support at most eight external
Go modules. The v0.1.0-alpha.3 implementation allows 128 selected modules,
but older readers must not be assumed to accept new selective-capture capsules.

The current reader has been tested against a previously valid
v0.1.0-alpha.2 schema-v3 capsule and reproduced its recorded failure.

Older readers may reject new capsules containing graph-only metadata,
even when the selected module count is within their supported limit.

## Go Dependency Restrictions

The v0.1.0-alpha.3 implementation supports up to 128 selected external Go
modules, including transitive dependencies, with at most 64 modules
requiring full ZIP artifacts. Required module metadata and ZIP artifacts
must already be available in the local Go module cache.

Unsupported or unverified configurations include:

- Dependency graphs containing more than 128 selected external modules
- Builds requiring more than 64 full ZIP modules
- Missing offline module-cache artifacts
- Workspace configurations using `go.work`
- Local replacements
- Unsupported `replace` or `exclude` directives
- Private module authentication workflows
- Vendor-only builds
- Arbitrary dependency cache formats

Dependency content is verified, but verification does not establish whether the dependency source code is trustworthy.

## Workspace Restrictions

The user selects files through `--include`.

Files referenced by Go `//go:embed` directives must be captured. For supported Go projects, `--include-go-embed` can discover and include embedded resources automatically; otherwise use explicit `--include` paths. Automatic discovery does not guarantee inclusion of every non-Go build input.

The resulting capsule does not automatically contain the entire repository or working environment.

Consequently:

- Omitted source files can prevent reproduction.
- Generated files are not automatically reconstructed.
- External filesystem paths are not fully captured.
- Local caches outside the supported dependency mechanism are not captured.
- Processes and background services are not snapshotted.
- The original operating system is not reproduced.

For supported schema-v3 captures, include `go.mod` and any existing `go.sum`. Dependency-free Go projects without `go.sum` are supported through an embedded virtual empty file.

## Go Toolchain Selection During Capture

For schema-v3 capture, REW resolves the `go` executable from `PATH`
once and uses the resulting absolute path for runtime detection,
environment inspection, source and embed discovery, dependency
analysis, and execution of the captured build.

These Go subprocesses run with `GOTOOLCHAIN=local`, overriding any
inherited `GOTOOLCHAIN` setting. Automatic Go toolchain switching
is therefore disabled during schema-v3 capture.

Resolving the executable once does not make its contents immutable.
REW does not protect against replacement or modification of the
selected Go executable between subprocess invocations.

Schema-v3 supports exactly Go 1.25.14 and Go 1.26.6.
Legacy schema-v2 retains its original Go 1.26.6 runtime policy.
Selecting the executable once does not remove the existing capture
consistency and trust-boundary limitations.

## Go Environment Restrictions

The supported portability configuration is deliberately constrained.

The tested cross-platform scenario uses:

```sh
GOOS=linux
GOARCH=amd64
CGO_ENABLED=0
GOFLAGS=
GOTOOLCHAIN=local
```

Different compiler flags, build tags, architecture-specific sources, and environment-sensitive build behavior may produce different results.

The current implementation does not reproduce every property of the original CI runner.

## Replay Restrictions

Replay requires a working Docker environment capable of executing Linux AMD64 containers.

Schema-v3 replay defaults to 256 MiB of container memory.
The --memory option accepts 256m, 512m, 1g, or 2g.

AMD64 emulation on ARM64 may require additional memory.
An out-of-memory failure can produce a different_failure outcome.
Increasing the limit may change the result, but does not guarantee reproduction.

The Docker image is pinned for reproducibility.

The replay container has network access disabled. However, the host may require network access to fetch the replay image.

The replay process may perform preparation steps, including offline module-cache setup, before running the captured build.

Therefore, replay should not be interpreted as an exact record-and-reexecute mechanism for every operation originally performed by Go.

## Failure Comparison

A reproduced outcome indicates agreement according to REW's implemented comparison criteria.

For the supported deterministic compiler-failure scenario, exit code and stderr are central to this comparison.

Known sources of divergence include:

- Non-deterministic diagnostics
- Environment-sensitive compiler output
- Missing selected source files
- Different build targets
- Unsupported module configurations
- Differences between host and container execution environments

A different result is not necessarily a REW defect; it can indicate that the captured environment was insufficient to reproduce the original failure.

## Security Limitations

REW is not currently a safe executor for arbitrary untrusted capsules.

Important limitations include:

- No authenticated capsule provenance.
- No guarantee that included source code is harmless.
- No comprehensive secret redaction.
- No complete protection against concurrent staging modification.
- No guarantee of isolation against container-runtime or kernel vulnerabilities.
- No complete protection against all resource-exhaustion attacks.

See `SECURITY.md` for the complete experimental security policy.

## Unsupported Claims

The current implementation does not claim:

- Complete CI job reproduction.
- General-purpose build-system virtualization.
- Reproduction of arbitrary Docker builds.
- Bit-for-bit rebuilding of successful binaries.
- Support for Kubernetes workloads.
- Arbitrary operating-system and architecture compatibility.
- Support for every Go module configuration.
- Safe execution of unknown third-party capsules.

These capabilities require separate engineering and validation.

## Before Public Alpha

The following work remains necessary before treating REW as a public alpha candidate:

1. Security review of capsule parsing and restoration.
2. Hardening staging and Docker execution boundaries.
3. Additional negative tests and failure-injection coverage.
4. Improved installation and release procedures.
5. Testing clean installations on supported platforms.
6. Versioned release artifacts and checksums.
7. Clear vulnerability reporting and maintenance procedures.

The current project should be treated as an experimental, limited-scope developer tool.
