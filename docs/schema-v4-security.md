# Schema V4 — Capture Security Boundaries

Status: experimental MVP.

## Supported scope

Schema V4 capture currently supports:

- A Go workspace with one sibling local module replacement.
- Exactly `go build ./...`.
- Explicit source inclusion through
  `rew capture --v4 --include-go-sources -- go build ./...`.
- An offline Linux/amd64 build target with CGO disabled.
- Portable `.rew` capsules containing the selected source files
  from the main and local modules.
- Offline Docker replay of supported capsules.

This is not a general-purpose snapshot of an arbitrary Go workspace.

## Source confidentiality

V4 capsules contain source file contents from both modules.

Capsules must be treated as potentially sensitive artifacts.
Review included files and captured stderr before sharing them.

The source-discovery and sensitive-path checks are defense-in-depth,
not a guarantee that all confidential material will be excluded.

## Pinned module roots

Capture uses a pinned local-module session with open directory
handles for the common parent, main module, and local module.

Root identity checks compare the pinned directories against their
current directory entries.

A persistent replacement of a module root is rejected even when the
replacement contains identical source bytes.

Root pinning does not create an atomic filesystem snapshot.

## Source integrity

Capture reads selected sources into memory before the build.

Following a failed build, it verifies the selected source paths,
normalized recorded modes, and bytes against that snapshot.

It also rechecks the selected module graph and verifies the source
snapshot again before packaging.

Tests confirm rejection of persistent source-content changes in
either the main module or the local replacement module.

Tests also confirm rejection of source replacement with different
bytes and of a persistent local-module root replacement.

## Known limitations

### No atomic snapshot

Verification observes filesystem state at discrete points.

A source file can potentially change during execution and be
restored before verification. Such intermediate changes are not
necessarily detected.

Therefore, the captured snapshot is not guaranteed to represent
every byte observed by the original Go compiler when the workspace
is concurrently writable.

### File identity

Selected regular-file identity is not retained and compared across
the complete capture lifecycle.

A file may be replaced by another inode containing identical bytes
and still pass verification. This behavior is confirmed by a V4
regression test.

This does not itself change the captured source bytes, but it means
V4 does not provide a file-identity continuity guarantee.

### Concurrently writable workspaces

Do not treat V4 as a security boundary against an adversary capable
of modifying workspace files during capture.

For stronger reproducibility, capture from a quiescent workspace
or an immutable filesystem snapshot prepared outside REW.

## Output behavior

Capture refuses to overwrite an existing output path, including
an existing symbolic link.

Successful output creation uses exclusive file creation.

This behavior does not amount to a complete guarantee against all
concurrent output-path replacement attacks.

## Replay

A verified V4 capsule can replay an eligible failed Go build in an
isolated Docker environment with the supported offline settings.

Successful replay demonstrates reproduction of the observed failure
under the tested conditions. It is not proof that the original build
environment was captured atomically.

## Security claims

It is accurate to describe V4 as:

"Capturing selected Go source files into a verifiable portable
artifact, with pinned module roots and before/after integrity
checks, for offline reproduction of supported build failures."

Do not describe V4 as providing:

- Atomic filesystem capture.
- A complete defense against concurrent workspace mutation.
- Continuous regular-file inode identity verification.
- Guaranteed inclusion of every input consulted by a Go build.
- Guaranteed reproducibility of every possible build failure.
