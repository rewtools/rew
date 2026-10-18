<div align="center">

  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/img/dark_logo.png">
    <source media="(prefers-color-scheme: light)" srcset="./assets/img/light_logo.png">
    <img src="./assets/img/light_logo.png" alt="REW" width="380">
  </picture>

  <h3>Capture a failed Go build. Replay the failure offline.</h3>

  <p>
    <img src="https://img.shields.io/badge/status-experimental-orange" alt="Experimental">
    &nbsp;
    <img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="Apache 2.0">
    &nbsp;
    <img src="https://img.shields.io/badge/open%20source-yes-brightgreen" alt="Open Source">
  </p>

  <p>
    Portable build failure capsules for reproducible debugging.
  </p>

</div>

REW is an experimental open-source CLI that packages a failed Go build, selected workspace files, and verified dependency artifacts into a portable `.rew` capsule.

The capsule can then be replayed inside a constrained Linux Docker container without network access from the container.

> **Status:** Experimental engineering prototype. Not production-ready.

## Demo

Real terminal recording of REW v0.1.0-alpha.5 capturing and replaying a controlled compiler failure in [spf13/cobra](https://github.com/spf13/cobra).

![REW capture and replay](assets/demo/rew-alpha5-demo.gif)

The compiler error was intentionally introduced for this demonstration. REW does not support arbitrary CI job replay.

## Why REW?

A build can fail in one environment and be difficult to reproduce in another.

REW explores a different debugging workflow:

1. Run a build through REW.
2. Capture a failed execution and its required inputs.
3. Transfer the resulting `.rew` capsule.
4. Verify the capsule and replay the build in Docker.
5. Compare the original and replayed failure.

The goal is to make certain build failures easier to investigate without repeatedly recreating the original environment.

REW does not guarantee that every build failure can be reproduced.

## Current Support

The schema-v3 prototype supports:

- Go 1.25.14 or Go 1.26.6 (exact patch versions).
- The exact command `go build ./...`.
- Up to 128 selected external Go modules, including transitive dependencies; up to 64 with full ZIP artifacts.
- Selected regular workspace files.
- Verified full `.zip`, `.mod`, and `.info` artifacts for build-required modules; metadata-only artifacts for other selected modules.
- SHA-256 and Go module checksum verification.
- Captured stdout, stderr, exit code, and build environment metadata.
- Offline replay inside a Linux AMD64 Docker container.
- Comparison of original and replayed failures.
- An explicit opt-in for replaying captured commands.

REW also retains older experimental capture/replay modes.

The schema-v3 format is experimental and may change.

**Capsule filename compatibility:** Schema-v3 capture creates `failure.rew`. Existing `.bfc` capsules remain readable by schema-v3 audit and replay. Legacy capture modes retain `failure.bfc`. The historical v0.1.0-alpha.5 binaries also create `.bfc` capsules.

Projects with no external Go dependencies are supported. For these
projects, the capsule contains no dependency artifacts.

The current implementation supports opt-in
`--include-go-sources` to automatically select main-module Go sources,
`go.mod`, and any existing `go.sum`. Manual `--include` selection
remains available. Dependency-free Go projects without `go.sum` are
supported: an empty file is embedded in the capsule.
Active Go workspaces (`go.work`) are unsupported; use `GOWORK=off`.

## Requirements

For development and the supported schema-v3 workflow:

- Building REW from source requires Go 1.26.6.
- Schema-v3 capture supports Go 1.25.14 or Go 1.26.6 (exact patch versions).
- Docker Engine or Docker Desktop with Linux containers.
- Linux AMD64 container execution support.
- Access to the pinned replay image when it is not already cached.

Replay containers run without network access. The Docker host may still need network access to obtain the pinned image.

## Install

The planned next REW prerelease is
[v0.1.0-alpha.6](https://github.com/rewtools/rew/releases/tag/v0.1.0-alpha.6).

The planned alpha.6 release supports schema-v3 capture and replay with Go 1.25.14 or Go 1.26.6 (exact patch versions).

The planned release targets macOS ARM64 and Linux AMD64.

After v0.1.0-alpha.6 is published, download and verify its binary:

```sh
VERSION=v0.1.0-alpha.6
BASE="https://github.com/rewtools/rew/releases/download/$VERSION"

case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) PLATFORM=darwin_arm64 ;;
  Linux/x86_64) PLATFORM=linux_amd64 ;;
  *)
    echo "Unsupported platform" >&2
    exit 1
    ;;
esac

mkdir rew-install
cd rew-install

curl -fL --retry 3 -O "$BASE/rew_${VERSION}_${PLATFORM}.tar.gz"
curl -fL --retry 3 -O "$BASE/SHA256SUMS"

grep -F "  ./rew_${VERSION}_${PLATFORM}.tar.gz" SHA256SUMS > selected.sha256
test -s selected.sha256

if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 -c selected.sha256
else
  sha256sum -c selected.sha256
fi

tar -xzf "rew_${VERSION}_${PLATFORM}.tar.gz"
./rew version

export REW_BIN="$(pwd)/rew"
```

For development builds, Go 1.26.6 is required:

```sh
git clone https://github.com/rewtools/rew.git
cd rew
go build -o rew ./cmd/rew
./rew version
export REW_BIN="$(pwd)/rew"
```

Development builds normally report `rew dev`.

SHA-256 verifies archive integrity against the downloaded manifest.
It does not independently authenticate the publisher.

## Terminal Branding

REW includes an optional terminal splash screen:

    rew splash

The command displays the REW ASCII logo, current CLI version,
and Docker engine label.

When stdout is an interactive terminal, REW uses its
purple, teal, and emerald TrueColor palette.

Colors are disabled when stdout is redirected or when
`NO_COLOR` is set, `CLICOLOR=0`, or `TERM=dumb`.

The splash screen is not printed by `capture`, `replay`,
or `version`, keeping normal CLI output suitable for scripts.

## Command-Line Help

After building REW, you can inspect its available commands:

    ./rew --help
    ./rew capture --help
    ./rew replay --help
    ./rew audit --help
    ./rew --version

The `capture` and `replay` help commands describe the supported schema-v3
workflow. Older experimental modes remain available but are not the
recommended starting point.

## Quick Start: Reproduce a Failed Build

This example requires Go 1.26.6, Docker with Linux AMD64
support, and the REW CLI installed above.

Create a dependency-free Go project:

```sh
mkdir rew-example
cd rew-example

cat > go.mod <<'MOD'
module example.com/rew-quickstart

go 1.26.0
MOD

cat > main.go <<'GO'
package main

func main() {
    undefinedFunction()
}
GO
```

Capture the intentional compiler failure:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS='' \
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
"$REW_BIN" capture --v3 \
  --include-go-sources \
  -- go build ./...
```

The build fails intentionally and REW saves `failure.rew`.
A successful capture normally returns the original build's
nonzero exit code.

No `go.sum` is required for this dependency-free example.
REW does not create `go.sum` in the source project.

Replay the failure using Docker:

```sh
"$REW_BIN" replay --v3 --allow-command --memory 1g failure.rew
```

Expected output:

```text
Original exit code: 1
Replay exit code:   1
Outcome:            reproduced
Replay memory:      1 GiB
```

Only replay capsules from trusted sources. Replay executes
the command recorded inside the capsule.

The [alpha.3 Quick Start validation report](docs/alpha3-quickstart-validation.md) documents an earlier maintainer-operated workflow. The v0.1.0-alpha.5 public binary has also been tested by the maintainers against a minimal Go project and a controlled compiler failure in spf13/cobra. This is not independent external-user validation.

## Automatic Go Source and Embed Discovery

The current implementation supports opt-in
`--include-go-sources` for supported Go projects.

For a supported Go project, run:

    "$REW_BIN" capture --v3 --include-go-sources -- go build ./...

This discovers selected main-module Go source files for Linux AMD64
and, on macOS ARM64, the capture platform. It also includes `go.mod`
and an existing `go.sum`. When `go.sum` does not exist, the existing
virtual-empty-file behavior remains available for dependency-free builds.

Manual `--include` paths can be combined with automatic discovery.
Paths discovered more than once are deduplicated; duplicate explicit
`--include` paths remain an error.

Embedded resources are separate. Add `--include-go-embed` to select
resources referenced by `//go:embed` directives:

    "$REW_BIN" capture --v3 --include-go-sources --include-go-embed -- go build ./...

The existing `--include-go-embed` flag also works with manual source
inclusion in the published release:

    "$REW_BIN" capture --v3 --include go.mod --include main.go --include-go-embed -- go build ./...

Discovery uses offline Go package metadata and does not download missing
dependencies. It selects files recognized by the supported Go toolchain;
it does not guarantee capture of every non-Go build input. CGO and
custom build tags are not supported in this workflow.

Discovery checks Linux AMD64 and also Darwin ARM64 when Capture uses
that source target. Results are deduplicated. Discovery is not sandboxed:
use it only with trusted source trees.

**Privacy:** selected source files, embedded resources, and explicit
includes become part of the `.rew` capsule. Review the contents and
avoid sharing capsules containing secrets or confidential source code.

The current schema-v3 workspace limit is
2,048 archived files, including a virtual `go.sum` when required.

## Understanding Replay Results

Make the `.rew` capsule available on a machine with a supported Docker environment.

Run:

```sh
"$REW_BIN" replay --v3 --allow-command failure.rew
```

When Replay v3 returns `different_failure` (exit code `11`), it also
reports the category of the mismatch:

- `exit codes differ`: the original and replayed commands exited with different codes.
- `stderr differs`: exit codes match, but the captured and replayed standard error differ.

For example:

    Original exit code: 1
    Replay exit code:   1
    Outcome:            different_failure
    Difference:         stderr differs

By default, REW does not print the full stderr difference.

To inspect the original and replayed stderr, use:

    rew replay --v3 --allow-command --show-stderr failure.rew
    rew replay --v3 --allow-command --memory 1g --show-stderr failure.rew

Diagnostic output is limited to 16 KiB per stderr stream.
Truncation is explicitly indicated. Stderr may contain sensitive data.

This option does not change the strict comparison, outcome, or exit code.

If the captured or replayed stderr was truncated, the comparison is
`inconclusive` rather than claiming a complete mismatch.

Schema-v3 replay uses 256 MiB of container memory by default.

For memory-intensive builds or AMD64 emulation on ARM64, use:

    "$REW_BIN" replay --v3 --allow-command --memory 1g failure.rew

Supported values: 256m, 512m, 1g, 2g. Maximum: 2 GiB.
This option applies only to schema-v3 replay.

Changing memory limits can change the replay outcome.
Other Docker security restrictions remain unchanged.

For rootless Docker, set DOCKER_HOST to the rootless Docker socket
if automatic connection fails.

A successfully reproduced failure reports output similar to:

```text
Original exit code: 1
Replay exit code:   1
Outcome:            reproduced
```

`reproduced` means REW's current comparison logic considers the captured and replayed failures equivalent.

It does not prove that every aspect of the original execution environment was reconstructed.

For schema-v3 replay, matching exit codes in the range `129–159` are
classified as `inconclusive`, even when stderr matches exactly.
These codes can represent signal termination under the common
`128 + signal` convention. Docker Exec does not reliably distinguish
signal termination from an explicit exit with the same code.

This is a conservative heuristic, not a signal or OOM detector.
An application that intentionally exits with a code in this range
may therefore also receive `inconclusive`. The shared comparison
logic used by older replay schemas remains unchanged.


**Only replay capsules from sources you trust.** The `--allow-command` option explicitly permits execution of the command recorded in the capsule.

## Advanced Example: External Go Dependency

The following example creates a small Go project with one external dependency and an intentional compiler error.

Create the project:

```sh
mkdir rew-example
cd rew-example

cat > go.mod <<'MOD'
module example.com/rew-example

go 1.26.0

require github.com/google/uuid v1.6.0
MOD

cat > main.go <<'GO'
package main

import (
    "fmt"

    "github.com/google/uuid"
)

func main() {
    fmt.Println(uuid.NewString())
    undefinedFunction()
}
GO
```

Download and verify the dependency before capture:

```sh
go mod download github.com/google/uuid@v1.6.0
go mod download
```

Make sure `go.sum` contains both the module and its `/go.mod` checksum entries.

Schema-v3 capture selects the `go` executable from `PATH` once
and reuses its absolute path throughout capture. It enforces
`GOTOOLCHAIN=local` for Go subprocesses, even if the inherited setting
allows automatic switching. The selected executable is not protected
against modification or replacement on disk.

For a portable Linux AMD64 build target, set:

```sh
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
export GOFLAGS=
export GOTOOLCHAIN=local
```

Disable module proxy and checksum database access during capture:

```sh
export GOPROXY=off
export GOSUMDB=off
```

Ensure no external Go workspace is active:

```sh
export GOWORK=off
```

Run the REW executable using the `REW_BIN` variable defined during installation:

```sh
"$REW_BIN" capture --v3 \
  --include go.mod \
  --include go.sum \
  --include main.go \
  -- go build ./...
```

Capture v3 checks the portable Go environment before executing the build.
Supported source targets are `darwin/arm64` and `linux/amd64`, with
`CGO_ENABLED=0` and empty `GOFLAGS`.

If the environment is unsupported, Capture exits with code `125`.
For example, on macOS where CGO is enabled by default, set
`CGO_ENABLED=0` before capture.

The output file `failure.rew` is never overwritten. If it already exists,
move or rename it before repeating Capture. REW checks this before
executing the build and retains exclusive file creation protection.

The build is expected to fail because `undefinedFunction` does not exist.

REW saves `failure.rew` in the current working directory when the failed build satisfies the supported capture requirements.

A successfully captured compiler failure normally preserves the original nonzero build exit code.

If capture itself fails, REW may exit with code `125` and will not produce a valid new capsule.


## What Is Inside a Capsule?

A schema-v3 `.rew` capsule contains:

- A JSON manifest describing the captured execution.
- Selected workspace files and their integrity metadata.
- Captured build environment information.
- Zero dependency artifacts for dependency-free builds, or three
  verified artifacts per build-required external Go module (up to 64 modules).
- Recorded command output and exit status.

The archive is verified before replay. Workspace and module-cache
snapshots are constructed from verified capsule contents and uploaded
to Docker-managed volumes, without host bind mounts for replay inputs.

Integrity verification is not the same as authenticating who created a capsule.

## Capsule Share Audit

The planned v0.1.0-alpha.6 release includes `rew audit failure.rew`.
The historical v0.1.0-alpha.5 binaries do not include this command.

It verifies schema-v3 capsule integrity and performs limited checks for
sensitive workspace filenames and known credential assignments in command
arguments, stdout, stderr, and verified workspace file contents.

```sh
rew audit failure.rew
```

Exit codes: 0 = no findings from supported checks; 2 = invalid arguments;
3 = potential sensitive data detected; 125 = verification or cleanup error.

Audit does not execute the captured command, invoke Go or start Docker.
Dependency ZIP contents are not scanned. It does not detect arbitrary
secrets, redact capsules, or authenticate their publishers.

Findings contain generic categories, not matched credential values.

**A successful audit does not prove that a capsule is safe to share.**
Review confidential material before publishing or transferring capsules.

## Security

REW replays commands from captured artifacts. It must not be treated as a safe executor for arbitrary, untrusted code.

The replay implementation applies container restrictions, including disabled container networking, but Docker isolation is not a complete security boundary.

Capsules may contain:

- Source code and other included files.
- Command arguments.
- Build output and diagnostics.
- Environment metadata.
- Dependency artifacts.

Do not capture confidential projects or production credentials without carefully reviewing the information that may be embedded in the capsule.

A capsule can pass integrity checks without being trustworthy.

For the schema-v3 replay snapshot trust assumptions, Docker volume boundaries, and remaining security limitations, see [Replay Snapshot Security Model](docs/security-model.md).

## Known Limitations

The current schema-v3 implementation:

- Supports only the exact command `go build ./...`.
- Requires Go 1.25.14 or Go 1.26.6 for schema-v3 capture (exact patch versions).
- Supports up to 128 selected external Go modules, including transitive dependencies, with at most 64 full ZIP modules.
- The current implementation supports opt-in
  `--include-go-sources` to select main-module Go source files,
  `go.mod`, and any existing `go.sum`. Manual inclusion remains
  supported. Dependency-free projects without `go.sum` are supported.
- Does not support active Go workspaces (`go.work`), `replace`,
  or `exclude` directives.
- Requires the selected module graph and all required full or metadata-only
  module artifacts to be available locally during offline capture.
- Rejects module graphs containing more than 128 selected external modules or builds requiring more than 64 full ZIP modules.
- Captures up to 128 additional graph-only `.mod` files, with a combined
  size limit of 1 MiB, when older module versions are needed for offline
  dependency graph reconstruction.
- Requires those additional `.mod` files to be present in the local Go
  module cache and verified against `/go.mod` entries in `go.sum`.
- Does not support arbitrary-sized dependency graphs.
- Does not support general-purpose CI job capture.
- Requires explicit inclusion of other workspace inputs not
  selected by Go source or embed discovery. Automatic discovery is
  opt-in and does not guarantee a complete source-tree snapshot.
  The published schema-v3 workspace limit is 2,048 archived files.
  Without `--include-go-embed`, embedded resources must be included
  manually. With that flag, main-module embedded resources are
  discovered automatically.
- The schema-v2 workspace limit remains 64 files.
- Does not create a complete filesystem or process snapshot.
- Does not guarantee reproduction across all host and target platforms.
- Does not authenticate capsule origin.
- Is not approved for executing untrusted capsules.
- Uses non-atomic workspace consistency checks during capture;
  concurrent changes may still escape detection.
- Uses verified TAR snapshots and Docker-managed volumes rather than host staging bind mounts in the active replay-v3 path. Concurrent-modification and broader trust-boundary risks still require further review.

Replaying a captured failure can produce a different result or fail when the supported reproducibility assumptions are not met.

Replay runs with `GOWORK=off`, empty `GOFLAGS`, `GOPROXY=off`,
`GOSUMDB=off`, and `GOTOOLCHAIN=local` in the restricted Docker
execution container.

A failure is reported as reproduced only when the replay exit code
and complete stderr match the captured failure exactly.

## Tested Environments

The GitHub Actions CI suite exercises:

- Go unit tests and static analysis.
- Linux AMD64 CLI compilation.
- Docker offline replay integration tests.
- macOS ARM64 Go capture.
- Linux Docker replay of a schema-v3 capsule created on macOS ARM64.
- Existing legacy capture/replay compatibility tests.

The schema-v3 cross-platform CI scenario targets Linux AMD64 during capture, transfers the capsule between independent runners, and replays it on Linux AMD64.

This does not establish general compatibility with every Go build target.

## Development

Run tests:

```sh
go test -count=1 ./...
```

Run static analysis:

```sh
go vet ./...
```

Run Docker integration tests with a working Docker daemon:

```sh
REW_DOCKER_TEST=1 go test -p 1 -count=1 -timeout 15m ./...
```

Build the CLI:

```sh
go build -o rew ./cmd/rew
```

## Project Status

REW is in active experimental development.

Public alpha releases are available for early testing. Further security review, documentation, installation testing, and compatibility validation remain necessary before a stable release.

The next priorities include:

1. Documenting the threat model and trust boundaries.
2. Hardening the replay staging and execution path.
3. Expanding reproducibility tests.
4. Improving failure diagnostics.
5. Expanding real-world Go project compatibility within bounded resource limits.

Features outside the documented supported scenarios should not be assumed to work.

## Contributing

REW welcomes early contributors interested in Go, DevOps, build reproducibility, container infrastructure, and developer tools.

Contributions can include bug reports, tests, documentation, code improvements, and technical discussions.

Looking for a place to start? Explore our [open issues](https://github.com/rewtools/rew/issues), especially those labeled `good first issue` or `help wanted`.

For larger changes, please open an Issue to discuss the approach before submitting a Pull Request.

Read [CONTRIBUTING.md](CONTRIBUTING.md) to get started.

## License

Licensed under the Apache License, Version 2.0.

See [LICENSE](LICENSE).
