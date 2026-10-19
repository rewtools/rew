# REW Schema V4: Local Go Module Replacements

Status: Draft — not implemented or released.

## 1. Goal

Reproduce supported failed Go builds that use local module
replacements, while retaining portable, verifiable, offline replay.

The initial target is a single main Go module with local `replace`
directives and a bounded set of external dependencies.

## 2. Non-goals

The initial implementation does not target:

- Active `go.work` workspaces.
- Arbitrary CI pipelines or Docker builds.
- Absolute local replacement paths.
- Host bind mounts during replay.
- Symlinks or special filesystem objects.
- Automatic capture of arbitrary parent directories.
- Full reproduction of arbitrary external services or secrets.

## 3. Compatibility

Schema v4 will use an explicit `schema_version` value of 4.

Existing schema-v3 capsules must remain readable and replayable.
The v3 writer, verifier, and replay behavior must not silently change.

The `.rew` extension remains the preferred capsule filename.
Legacy `.bfc` reading compatibility must remain intact.

Older REW versions are not expected to understand schema v4.

## 4. Proposed Snapshot Model

The main module and local replacement modules are represented
inside one verified snapshot root.

Illustrative replay layout:

    /workspace/
      app/
        go.mod
        main.go
      lib/
        go.mod
        lib.go

The main module's execution directory is `/workspace/app`.

An original directive such as:

    replace example.com/lib => ../lib

continues to resolve without rewriting `go.mod`.

Exact internal directory names are implementation details.
The manifest must carry canonical, relative paths.

## 5. Proposed Manifest Extensions

Proposed fields, subject to implementation review:

- `execution.working_dir`: reuse the existing execution field,
  with schema-v4-specific validation requiring a canonical path
  to the main module relative to the snapshot root. No new
  top-level `working_dir` field is introduced.
- `local_modules`: bounded list of captured local module
  replacements with original module identity, replacement
  directive identity, canonical snapshot path, and file metadata.
- Extended selected Go module graph metadata identifying
  replacement relationships.

Every captured file must have a size and SHA-256 digest.

No absolute source-host paths may be required for replay.

The final JSON structure and archive entry ordering remain TBD.

## 6. Path Preservation

Capture must preserve the meaning of supported relative
replacement directives without modifying original `go.mod` bytes.

Capture must resolve and validate every replacement root.

The resulting snapshot paths must remain inside one bounded
virtual root and must not collide.

An unresolved, ambiguous, unsafe, or unsupported layout
must cause capture to fail before publishing an archive.

Absolute host paths must never be interpreted as portable
replacement locations.

## 7. Filesystem and Integrity Rules

Local module input files must be explicitly bounded and verified.

Requirements:

- Canonical relative archive paths.
- Regular files only.
- No symlink traversal.
- No absolute paths or path traversal in archive entries.
- Per-file SHA-256 verification.
- Duplicate and overlapping path rejection.
- Bounded file counts, depth, individual size, and total size.
- Verification before Docker execution.
- No rereading mutable host staging paths after verification.

The numeric limits remain to be designed and tested.

## 8. Replay

Schema-v4 replay should preserve the current verified
snapshot-to-Docker-volume architecture where practical.

The workspace volume is mounted read-only.
The module-cache source volume is mounted read-only.
Go build caches remain in bounded writable tmpfs mounts.

Replay must retain:

- Linux AMD64 runtime policy.
- Non-root execution.
- Disabled networking.
- Dropped capabilities and no-new-privileges.
- Read-only root filesystem.
- Resource limits.
- Ownership-checked Docker cleanup.

Replay must reject malformed or incomplete replacement
metadata before executing the captured command.

## 9. Go Module Graph Verification

The selected graph must account for both:

- Original required module path and version.
- Effective local replacement path and module content.

Go graph information must be compared against verified
manifest metadata before executing the recorded build.

Unverified local module content must not satisfy a replacement.

The graph comparison algorithm and canonical representation
remain to be specified.

## 10. Capture Consistency

Capture must inspect local module inputs before packaging.

Changes during capture, including changed `go.mod` or source
files, must not silently produce a capsule combining
inconsistent filesystem states.

The detailed snapshot and race-mitigation algorithm
remains to be specified.

## 11. Required Test Coverage

Before release:

- Dependency-free schema-v4 build.
- One local replacement.
- Multiple local replacements.
- Nested relative replacement paths.
- Mixed local and external modules.
- Missing replacement root.
- Changed replacement source during capture.
- Malformed, duplicate, and overlapping paths.
- Symlink and archive traversal rejection.
- Corrupt file checksum.
- Unexpected or mismatched Go module graph.
- Offline Linux AMD64 Docker reproduction.
- Non-root and read-only mount enforcement.
- Cancellation and resource cleanup.
- Existing schema-v3 regression tests.
- Legacy `.bfc` compatibility tests.

## 12. Open Design Questions

The following must be resolved before implementation:

1. Exact v4 manifest and archive structure.
2. Canonical virtual-root placement algorithm.
3. How to represent version-specific replace directives.
4. Module graph comparison with local replacements.
5. File discovery and size budgets for local module trees.
6. Snapshot consistency and race handling.
7. CLI version selection and backward compatibility.
8. Production-equivalent Docker end-to-end tests.

## 13. Evidence

Local experiments conducted on 2026-10-18 demonstrated:

- An original relative replacement builds offline.
- Moving the replacement without preserving its path breaks
  the build.
- A nested main-module layout preserves `../lib`.
- Missing replacement roots cause Go module graph resolution
  to fail.
- A Linux AMD64 Docker execution successfully resolves
  and builds a local replacement from a read-only volume.

These experiments establish feasibility for the narrow case.
They do not establish complete schema-v4 correctness or safety.

## 14. Design Invariants

### 14.1 Source Paths Versus Archive Paths

A Go local replacement directive may legitimately contain `..`.

For example:

    replace example.com/lib => ../lib

This source directive must not be rejected merely because it
contains a parent-directory component.

Instead, Capture must resolve it relative to the main module's
go.mod directory and map it into a bounded virtual snapshot root.

All resulting archive entry paths must be canonical, relative,
and free of parent-directory traversal.

No original absolute host path may be required for Replay.

### 14.2 Declared Versus Effective Replacements

The manifest design must distinguish:

- Replacement directives declared in go.mod.
- Replacements effective in the selected Go module graph.
- The verified source tree associated with each effective
  local replacement.

Version-specific directives may be present but inactive.

Capture must not incorrectly treat every declared directive
as an active build dependency.

Unsupported declarations must be handled explicitly rather
than silently discarded.

### 14.3 Content Integrity Versus Capture Completeness

Per-file hashes establish integrity of recorded bytes.

They do not establish that all files required by the original
build were captured.

Capture must define a bounded source-discovery policy for
local replacement modules, including relevant Go files,
go.mod files, and embedded resources.

A capsule must not claim complete reproducibility solely
because every recorded file passes checksum verification.

### 14.4 Fail-Closed Replay

Replay must validate replacement metadata, file integrity,
path mappings, and effective Go module graph before executing
the captured build command.

A missing local module, mismatched replacement path, invalid
manifest, or corrupted source file must produce an explicit
verification failure.

Replay must not silently fall back to the network, host
filesystem, or unverified module content.

### 14.5 Compatibility Boundary

Existing schema-v3 readers and archives must remain unchanged.

Schema-v4-specific replacement semantics must not be
introduced as fields that older schema-v3 readers silently
ignore while treating the capsule as fully understood.

The format version is a semantic compatibility boundary,
not merely a manifest field added for convenience.

## 15. First Implementation Slice

### 15.1 Scope

The first implementation slice targets:

- One main Go module.
- Exactly one effective local directory replacement.
- Relative local replacement paths only.
- An original, unmodified main-module go.mod.
- Offline Linux AMD64 reproduction of a failed Go build.
- Existing external Go dependencies where supported by v3.

Support for multiple local replacements is deferred.

### 15.2 Required Behavior

Capture must:

1. Identify the effective local replacement.
2. Resolve the replacement relative to the main go.mod.
3. Validate both source roots and their selected files.
4. Construct a canonical virtual-root layout.
5. Preserve relative Go replacement semantics.
6. Record sufficient metadata to verify the effective graph.
7. Detect inconsistent source content before publication.
8. Reject incomplete or unsupported configurations.

Replay must:

1. Verify the schema-v4 capsule and file checksums.
2. Construct bounded TAR snapshots from verified bytes.
3. Transfer snapshots into Docker-managed volumes.
4. Mount the workspace and module cache read-only.
5. Execute from the recorded main-module directory.
6. Resolve and verify the effective Go module graph.
7. Execute the recorded failing build only after verification.
8. Compare replay failure characteristics with capture.

### 15.3 Required End-to-End Fixture

The first integration fixture contains:

    project/
      app/
        go.mod
        main.go
      lib/
        go.mod
        lib.go

The main go.mod contains:

    require example.com/local-lib v0.0.0
    replace example.com/local-lib => ../lib

The main build contains a deterministic compilation failure.

Capture must produce a verifiable schema-v4 .rew capsule.

Offline Docker Replay must reproduce the recorded failure
without network access or host workspace bind mounts.

### 15.4 Completion Criteria

The first slice is complete only when:

- Capture and Replay succeed end-to-end.
- The expected failure is reproduced.
- The original go.mod is preserved byte-for-byte.
- Replacement source corruption is rejected.
- Missing replacement content is rejected.
- Graph mismatch is rejected before build execution.
- Existing schema-v3 regression tests pass.
- Legacy .bfc compatibility tests pass.
- Docker execution security checks pass.

### 15.5 Implementation Ordering

Implementation should proceed in small changes:

1. Finalize the v4 manifest and archive contract.
2. Implement local replacement metadata validation.
3. Implement bounded capture and virtual-root mapping.
4. Implement schema-v4 archive writing and verified reading.
5. Implement verified snapshot preparation.
6. Extend offline graph verification for replacements.
7. Wire schema-v4 CLI capture and replay.
8. Add production-equivalent Docker end-to-end tests.

Do not publish schema-v4 capsules before the complete
capture, verification, and replay path passes testing.
