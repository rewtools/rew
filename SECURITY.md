# REW Security Policy

## Project Status

REW is an experimental infrastructure tool.

It is not production-ready, has not undergone a comprehensive security audit, and must not be used as a security boundary for arbitrary or untrusted workloads.

No stable release is currently supported.

## Supported Security Scope

The current schema-v3 implementation focuses on verifying captured inputs and restricting replay execution.

It includes:

- Archive structure and metadata validation.
- SHA-256 verification of captured workspace files and dependency artifacts.
- Go module ZIP and go.sum checksum verification.
- Construction of workspace and module-cache TAR snapshots from verified workspace bytes and disk-backed dependency artifacts.
- Docker-managed snapshot volumes mounted read-only for execution.
- No host workspace bind mounts in the active schema-v3 replay path.
- Explicit authorization through `--allow-command`.
- Docker-based execution with restricted privileges.
- Disabled container networking.
- Read-only container root filesystem.
- Reduced Linux capabilities.
- Resource limits and execution timeouts.
- Cleanup of temporary execution containers.
- Best-effort cleanup of Docker snapshot volumes and preparation containers.

These controls reduce specific risks. They do not establish complete isolation or prove the absence of vulnerabilities.

## Threat Model

### Assets to Protect

The primary assets are:

- The developer's host filesystem.
- Source code and private project data.
- Credentials and environment secrets.
- Docker daemon access.
- Host CPU, memory, and storage.
- The integrity of captured build artifacts.

### Trust Boundaries

REW crosses several trust boundaries:

1. **Capture host to capsule:** selected files, command metadata, and diagnostic output are serialized into a capsule archive (`.rew` for new schema-v3 captures; legacy `.bfc` remains supported).
2. **Capsule to replay process:** archive contents are parsed and
   verified; dependency artifacts are held in a private disk-backed
   store, while workspace bytes remain bounded in memory before TAR
   snapshot construction.
3. **Replay process to Docker:** verified snapshots are uploaded into Docker-managed volumes and mounted read-only in the execution container.
4. **Container to host kernel:** containerized code executes using the host's container runtime and kernel interfaces.

Each boundary requires independent security consideration.

## Trusted and Untrusted Inputs

### Trusted Inputs

The current experimental workflow assumes:

- The user trusts the capsule's source.
- The capture environment is under the user's control.
- The Docker host and daemon are trusted.
- The selected replay image is obtained from a trusted registry.
- The host operating system and Docker runtime are correctly maintained.

### Untrusted Inputs

Potentially dangerous inputs include:

- Capsule files received from unknown parties.
- Captured source code.
- Go module archives and source files.
- Recorded execution arguments.
- Build diagnostics containing attacker-controlled text.
- Concurrent modification of capture inputs or replay-owned temporary files by processes with sufficient filesystem access.

**Do not replay capsules from unknown or untrusted sources.**

## Capsule Integrity

Schema-v3 performs multiple integrity checks before replay.

These checks are intended to detect malformed archives and modifications to recorded inputs.

However:

- SHA-256 does not authenticate the capsule publisher.
- Go module checksums validate content, not the trustworthiness of source code.
- A valid archive can contain malicious code.
- A valid manifest can describe an intentionally harmful execution.
- Integrity checking does not prevent every possible race or runtime exploit.

REW does not currently provide signed capsules, trusted publisher identities, or an authenticated provenance system.

## Docker Isolation

Replay uses Docker with restrictive runtime settings.

These include disabled container networking, a read-only root filesystem, dropped Linux capabilities, non-root execution, and other resource and privilege restrictions.

Important limitations:

- Containers share the host kernel.
- Docker daemon access is highly privileged.
- Container isolation is not equivalent to a dedicated virtual machine.
- Runtime or kernel vulnerabilities could undermine isolation.
- Resource limits do not eliminate every denial-of-service risk.
- The replay host may access the network to retrieve the pinned image, even though the replay container has networking disabled.

Do not use REW as a sandbox for hostile code.

## Host Filesystem and Replay Snapshots

The active schema-v3 replay path constructs workspace and module-cache
TAR snapshots from verified workspace bytes in memory and dependency
artifacts in a private disk-backed store. Prepared TARs are held in
private temporary files and consumed through open readers.

These snapshots are uploaded into Docker-managed volumes rather than
being restored into host staging directories for execution.

The execution container mounts both volumes read-only and does not
inherit host workspace bind mounts.

This removes the previous host-staging filesystem race between
verification and Docker consumption.

However, snapshot volumes are not absolutely immutable. The Docker
daemon and sufficiently privileged actors may modify them.

Resource cleanup is best-effort. Docker API failures, daemon
unavailability, or unexpected process termination may leave resources
that require manual inspection.

The snapshot architecture does not provide protection against
compromised Docker infrastructure, host-kernel vulnerabilities,
or malicious source code with valid checksums.

See [Replay Snapshot Security Model](docs/security-model.md) for
implementation details and remaining limitations.

## Dependency Handling

Schema-v3 supports bounded selection of external Go modules,
including transitive dependencies: up to 128 selected external modules,
of which up to 64 may have complete ZIP artifacts.

Dependency artifacts are captured from the Go module cache and checked
against expected metadata and checksums.

Replay packages verified dependency artifacts into a module-cache snapshot.
The execution container copies these artifacts from a read-only Docker
volume into writable tmpfs and performs Go module preparation offline.

Limitations include:

- Not every Go module graph or build configuration is supported.
- Required dependency artifacts must be available during capture.
- Captured dependencies are not guaranteed free of vulnerabilities.
- Module checksums do not authenticate the capsule publisher.
- Replay must not execute untrusted source code.

## Secrets and Sensitive Information

Capsules may include:

- Source code and explicitly included workspace files.
- Captured stdout and stderr.
- Command arguments.
- Build environment metadata.
- Dependency contents.

Sensitive data may appear in logs, arguments, or source files even when it was not intentionally selected for disclosure.

Do not upload or share `.rew` or `.bfc` capsules containing confidential information.

The development-source audit command provides limited checks for
verified schema-v3 capsules. It examines sensitive workspace filenames
and known credential assignments in recorded argv, stdout, stderr,
and verified workspace file contents. Findings contain generic categories.

Exit codes: 0 (no findings), 2 (invalid arguments), 3 (potential sensitive
data), and 125 (verification or cleanup failure).

It does not execute the recorded build, require Docker, or scan dependency
ZIP contents. Other sensitive information may remain undetected.
Zero findings does not establish that sharing is safe.
The historical alpha.5 binaries do not include this command. The published alpha.6 release includes it.

REW does not currently promise comprehensive secret detection or redaction.

## Safe Usage Guidelines

For the current experimental version:

1. Capture only projects and files you are authorized to handle.
2. Review included files and recorded diagnostics before sharing capsules.
3. Replay only capsules from sources you trust.
4. Do not run REW with production credentials.
5. Avoid privileged Docker configurations.
6. Keep Docker, the operating system, and Go toolchain updated.
7. Use a dedicated disposable development environment for higher-risk testing.

## Reporting Security Vulnerabilities

Please do not publicly disclose an exploitable vulnerability before giving maintainers an opportunity to investigate.

If private vulnerability reporting is enabled for this GitHub repository, use its **Security → Report a vulnerability** interface.

If private reporting is unavailable, avoid posting exploit details or sensitive information in a public issue. Contact the repository maintainers through an appropriate private channel.

No guaranteed response time or formal security support SLA is currently offered.

## Security Roadmap

Before a stable release, the project should continue addressing:

- Independent review of archive parsing and extraction.
- Further review of concurrent modifications, snapshot integrity, and filesystem race boundaries.
- Additional negative and adversarial tests.
- Resource exhaustion and cancellation edge cases.
- Dependency graph and artifact validation edge cases.
- Replay image and supply-chain verification.
- Clear release and vulnerability handling procedures.
- Evaluation of stronger isolation for untrusted workloads.

## Disclaimer

REW is provided as experimental software under the repository's license.

Do not interpret successful tests, checksums, or Docker restrictions as a guarantee that replaying a capsule is safe.
