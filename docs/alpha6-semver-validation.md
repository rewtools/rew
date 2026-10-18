# REW v0.1.0-alpha.6 — Masterminds/semver Validation

## Scope

This report documents a maintainer-operated validation of the
published REW v0.1.0-alpha.6 macOS ARM64 binary against a pinned
third-party Go project.

The compiler failure was intentionally introduced into a separate
fixture copy. It was not an upstream Masterminds/semver defect.

## Environment

- REW release: v0.1.0-alpha.6
- REW binary: official macOS ARM64 release archive
- Release archive: verified against published SHA256SUMS
- Capture host: macOS ARM64
- Go toolchain: Go 1.26.6
- Build target: Linux AMD64
- CGO_ENABLED: 0
- Docker Engine: 29.6.2
- Replay memory limit: 1 GiB
- Replay container networking: disabled by REW's snapshot runtime

## Upstream Project

- Repository: https://github.com/Masterminds/semver
- Commit: dd2b995c61c39ddd668b23ac919b04d607be35ab
- Module: github.com/Masterminds/semver/v3
- go.mod directive: go 1.21
- Go source files found: 8
- External requirements in go.mod: none
- Root go.work: absent
- Nested Go modules: none

The original checkout passed an offline Linux AMD64
`go build ./...` baseline and remained unchanged.

## Test Scenario

A separate source fixture was created from the pinned upstream
commit. The following Go file was added:

    package semver

    var rewRegressionFailure = rewIntentionallyUndefinedSymbol

The official REW binary executed schema-v3 Capture with
`--include-go-sources` and the exact command `go build ./...`.

Capture used Linux AMD64 build settings, CGO disabled, and
Go module networking disabled.

## Observed Results

| Check | Result |
| --- | --- |
| Capture exit code | 1 |
| Selected Go source files | 5 |
| Capsule filename | failure.rew |
| Schema version | 3 |
| Archived workspace files | 7 |
| External Go modules | 0 |
| Capsule size | Approximately 57 KiB |
| Capsule integrity audit | Verified |
| Sensitive-data audit findings | 0 |
| Original build exit code | 1 |
| Replay build exit code | 1 |
| Replay outcome | reproduced |
| Replay CLI exit code | 0 |
| Original and replay stderr | Identical in displayed diagnostics |

The original and replayed compiler error was:

    # github.com/Masterminds/semver/v3
    ./rew_regression_failure.go:3:28: undefined: rewIntentionallyUndefinedSymbol

The capsule SHA-256 was:

    d70e4510bf51d08db489c82c499ff8bee04d23daf7af1f7b7e3df5e00afe1525

The same SHA-256 was verified before and after replay.

## Interpretation and Limitations

This establishes successful Capture, integrity verification,
and offline Linux AMD64 Docker reproduction of one controlled
compiler failure using the official alpha.6 binary.

Capture and replay ran on the same physical computer, with the
replay build running in Linux AMD64 Docker.

This is not independent external-user validation and does not
establish reproduction of a naturally occurring upstream defect.

The fixture had no external Go dependencies, so this experiment
does not validate dependency-cache reconstruction or complex
Go module graphs.

A zero-finding sensitive-data audit does not guarantee that a
capsule is safe to share. Replay is not safe for untrusted
capsules solely because Docker isolation is used.

No universal Go compatibility, full CI reproduction, or
production readiness is claimed.
