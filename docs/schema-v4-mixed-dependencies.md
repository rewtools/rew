# Schema V4: Mixed Local and External Go Dependencies

Status: Experimental initial slice implemented in development
source; not included in the published v0.1.0-alpha.6 binaries.

Implementation evidence includes real failed-build capture,
complete archive verification, CLI dispatch to local-only or
mixed replay, and offline Docker reproduction. The supported
scope remains exactly one sibling local replacement and one
external versioned Go module. This document also retains
historical engineering-contract language describing the design
and test requirements; it is not a claim of general graph support.

## 1. Objective

Extend experimental Schema V4 to capture and replay a failed
`go build ./...` when the selected Go module graph includes:

- One sibling local module replacement, such as `../lib`.
- One external versioned module required by the build.
- A Linux/amd64 target with CGO disabled.

The external module and its verification artifacts must already
exist in the local Go module cache. No downloads are permitted.

## 2. Compatibility

- Preserve existing Schema V3 capture, verification, and replay.
- Preserve legacy `.bfc` reading compatibility.
- Preserve existing workspace-only Schema V4 capsules.
- Do not change previously published binaries.
- Keep `schema_version: 4` for this incremental extension.
- Extend the V4 archive contract explicitly; never reinterpret
  existing workspace entries as dependency artifacts.
- Any incompatibility discovered during implementation requires
  a separate design decision before proceeding.

## 3. Initial Scope

- Exactly one unversioned sibling local replacement.
- Exactly one external versioned module with full artifacts.
- No additional transitive external modules in this slice.
- No active `go.work`, external services, or arbitrary CI jobs.
- No versioned, absolute, or nested local replacement paths.
- Existing V4 source-discovery and capture-consistency limits
  continue to apply.

This is the first supported slice, not a claim of general
mixed-module-graph completeness.

## 4. Graph Contract

- Preserve the effective selected module path and version.
- Preserve the local replacement identity and relative path.
- Distinguish local replacements from external modules.
- Validate the main `go.mod` against the selected graph.
- Validate local module identity from its captured `go.mod`.
- Require a complete, explicitly bounded external module set.
- Reject missing, duplicate, extra, or mismatched modules.
- Account for module requirements of the local replacement.
- Do not treat partial `go list` output as complete evidence.
- Recheck the selected graph after execution.

## 5. External Artifact Contract

For the initial external module require:

- One `.zip` containing validated module content.
- One `.mod` matching the module identity.
- One `.info` matching the selected version.
- SHA-256 metadata for every archived artifact.
- Valid ZIP structure and bounded unpacked size.
- Matching ZIP and go.mod hashes in captured `app/go.sum`.
- Offline availability of all required artifacts.

Reuse existing V3 verification primitives where their
preconditions hold. Do not bypass or weaken their checks.

## 6. Archive Layout

Existing V4 archives contain:

- `manifest.json`
- `workspace/*`

The extension proposes:

- `manifest.json`
- `workspace/*`
- `dependencies/*`

The final dependency entry naming, ordering, and manifest
mapping must be specified and tested before implementation.

Both old workspace-only and new extended V4 capsules must
be accepted by the updated reader.

Unknown, duplicate, unexpected, or extra TAR entries must
be rejected. Archive-size preflight must include dependencies.

## 7. Replay Trust Boundary

- Verify the complete capsule before invoking Docker.
- Verify the external module set against the manifest and
  the captured go.sum.
- Keep large module artifacts in a bounded disk-backed store.
- Generate complete workspace and module TAR streams before
  starting Docker.
- Recheck artifact content during snapshot serialization.
- Pass only completed, verified snapshots to Docker.
- Maintain read-only volume mounts and disabled networking.
- Preserve resource limits, non-root execution, and cleanup.
- Do not reread mutable host Go module cache paths in Replay.

## 8. Capture Consistency

- Preserve V4 pinned module-root handling.
- Preserve before/after selected workspace byte checks.
- Confirm captured `app/go.mod` and `app/go.sum` are consistent
  with the selected external module set.
- Reject an absent or unverified external module.
- Reject changes to selected module graph or source bytes
  detected during capture.
- Keep the documented non-atomic snapshot limitation.

## 9. Required Negative Tests

Before enabling mixed-dependency capture and replay:

1. Missing external ZIP, mod, or info.
2. Corrupt ZIP and ZIP traversal or symlink entries.
3. Incorrect external module path or version.
4. Wrong or missing go.sum checksum.
5. Extra or missing selected external module.
6. Duplicate or out-of-order archive entries.
7. Extra undeclared dependency artifact.
8. Oversized compressed or unpacked dependency.
9. Changed cached artifact during snapshot preparation.
10. Changed local or main Go sources during capture.
11. Changed selected Go module graph during capture.
12. Corrupt manifest or external artifact metadata.
13. Old workspace-only V4 capsule regression.
14. Schema V3 and legacy `.bfc` regression.
15. Offline Docker replay of the expected build failure.
16. Verification failure before Docker invocation.
17. Cancellation and Docker resource cleanup.

## 10. Implementation Order

1. Validate the design against existing V3 and V4 contracts.
2. Specify mixed-module metadata and archive ordering.
3. Implement failing unit tests for the new validation rules.
4. Implement bounded external artifact capture.
5. Extend the V4 writer and verified reader.
6. Build verified module-cache snapshots for Replay.
7. Connect Capture and Replay behind the existing V4 flag.
8. Complete security, compatibility, and real Docker E2E tests.
9. Update public documentation and reassess release readiness.

No feature is considered supported merely because its manifest
fields exist or a basic fixture can be replayed.
