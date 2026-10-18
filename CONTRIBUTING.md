# Contributing to REW

Thank you for your interest in contributing to REW!

REW is an experimental open-source developer tool for capturing failed Go builds and replaying them offline in Docker.

The project is under active development. Contributions of all sizes are welcome, including bug reports, tests, documentation, code improvements, and thoughtful technical discussions.

## Before You Start

Please read:

- [README.md](README.md) — project overview and quick start
- [Architecture](docs/architecture.md) — current implementation
- [Limitations](docs/limitations.md) — supported and unsupported scenarios
- [Security Policy](SECURITY.md) — trust boundaries and security considerations

REW is not production-ready. Please avoid assuming that unsupported commands, environments, or dependency configurations work.

## Ways to Contribute

You can help by:

- Reporting reproducible bugs
- Improving documentation
- Writing unit and integration tests
- Improving error messages and developer experience
- Investigating portability and reproducibility issues
- Reviewing code and proposed designs
- Contributing Go, Linux, Docker, or CI expertise

Look for GitHub Issues labeled `good first issue` or `help wanted`.

If no suitable issue exists, feel free to open one describing your proposal.

## Development Setup

### Requirements

- Go 1.26.6
- Git
- Docker Engine or Docker Desktop for integration tests
- Linux AMD64 container execution support for replay tests

### Clone and Build

Fork the repository on GitHub, then clone your fork:

```sh
git clone https://github.com/YOUR_USERNAME/rew.git
cd rew
```

Build the CLI:

```sh
go build -o rew ./cmd/rew
./rew version
```

The development version currently reports `rew dev`.

### Run Tests

Run the ordinary Go test suite without requiring Docker:

```sh
go test -count=1 ./...
```

Docker-dependent tests are skipped unless explicitly enabled.

Run static analysis:

```sh
go vet ./...
```

Check formatting:

```sh
find cmd internal -name '*.go' -type f -exec gofmt -l {} +
```

Run Docker-dependent integration tests when Docker is available:

```sh
REW_DOCKER_TEST=1 go test -p 1 -count=1 -timeout 15m ./...
```

Docker-dependent tests require a running Docker daemon and Linux AMD64
container execution support. These tests may pull the pinned replay image,
so the host may need network access before offline replay can run.
Docker is not required to build the CLI or run the ordinary test suite.

## Development Workflow

1. Find or open a GitHub Issue describing the proposed change.
2. Discuss the approach first if the change is significant.
3. Create a feature branch from the current `main`.
4. Make a focused change with appropriate tests.
5. Run relevant tests and static analysis.
6. Open a Pull Request referencing the Issue.
7. Address review comments before merging.

Example:

```sh
git checkout main
git pull --ff-only origin main
git checkout -b fix/improve-error-message
```

Avoid committing directly to `main` when preparing a contribution.

## Pull Request Guidelines

A good Pull Request should:

- Describe the problem being solved.
- Explain the proposed implementation.
- Reference related Issues.
- Include tests for behavioral changes.
- Update documentation when behavior changes.
- Keep unrelated changes out of the PR.
- Pass relevant GitHub Actions checks.

Small, focused Pull Requests are preferred over large, unrelated changes.

Please do not submit large code rewrites without prior discussion.

## Code Style

REW is written primarily in Go.

Please:

- Follow standard Go conventions.
- Format Go code with `gofmt`.
- Handle errors explicitly.
- Prefer clear, maintainable code over unnecessary abstractions.
- Avoid adding dependencies without a documented reason.
- Keep behavior deterministic where practical.
- Avoid introducing network requirements into offline replay.
- Add regression tests when fixing bugs.

## Security-Sensitive Changes

Some parts of REW require additional review:

- Capsule archive parsing and verification
- Workspace restoration and staging
- Filesystem path validation
- Go module artifact verification
- Docker container configuration and execution
- Privilege and resource restrictions
- Command authorization
- Replay image selection
- Trust and portability validation

Before working on a significant change in these areas, open an Issue describing the intended design, unless doing so would disclose a security vulnerability.

Security-sensitive changes should include threat-model considerations and negative tests where appropriate.

Do not weaken existing validation or isolation controls merely to make a test pass.

## Reporting Security Vulnerabilities

Do not publish exploitable security vulnerabilities in public Issues or Pull Requests.

Please follow the private reporting guidance in [SECURITY.md](SECURITY.md).

## Compatibility

Schema-v3 intentionally supports a narrow Go build scenario.

Changes that expand supported commands, module configurations, platforms, or execution environments must include corresponding validation rules and tests.

Do not silently broaden compatibility assumptions.

The capsule schema is experimental, but schema changes still require discussion and review. The `.rew` and legacy `.bfc` filename extensions do not determine the internal schema version.

## Documentation

Documentation is written in English.

Please keep documentation consistent with implemented behavior.

Do not describe planned features as already supported.

Examples should be runnable on their documented supported platforms.

## Code of Conduct

Be respectful and constructive in Issues, Discussions, code reviews, and Pull Requests.

Focus feedback on technical decisions rather than individuals.

A formal Code of Conduct may be introduced as the contributor community grows.

## Maintainer Review

Submitting a contribution does not guarantee acceptance or a particular review timeline.

Maintainers may request changes, additional tests, or design discussion.

The goal is to maintain a reliable, understandable, and secure foundation for REW.

Thank you for helping improve REW!
