package replay

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/runtime/docker"
	"golang.org/x/mod/modfile"
)

// RunV4WithOptions replays a verified schema-v4 capsule with
// exactly one locally replaced Go module.
//
// Experimental: Docker Exec is not equivalent to PID 1.
// The capsule is verified before its contents reach Docker.
// Docker execution requires explicit opt-in.
func RunV4WithOptions(
	ctx context.Context,
	capsulePath string,
	options Options,
) (Result, error) {
	var result Result

	if options.MemoryBytes != 0 {
		switch options.MemoryBytes {
		case 256 << 20, 512 << 20, 1 << 30, 2 << 30:
		default:
			return result, fmt.Errorf(
				"replay: invalid v4 memory limit: %d bytes",
				options.MemoryBytes,
			)
		}
	}

	verified, err := capsule.ReadVerifiedV4(capsulePath)
	if err != nil {
		return result, fmt.Errorf(
			"replay: verify v4 capsule: %w", err,
		)
	}

	manifest := verified.Manifest

	if manifest.Platform.OS != "linux" ||
		manifest.Platform.Architecture != "amd64" {
		return result, fmt.Errorf(
			"replay: unsupported v4 platform %s/%s",
			manifest.Platform.OS,
			manifest.Platform.Architecture,
		)
	}

	if !options.AllowArbitraryCommand {
		return result, fmt.Errorf(
			"replay: v4 command execution requires explicit opt-in",
		)
	}

	if manifest.Runtime == nil ||
		manifest.Runtime.Kind != "go" ||
		!capsule.IsSupportedGoVersion(manifest.Runtime.Version) {
		return result, fmt.Errorf(
			"replay: unsupported v4 Go runtime",
		)
	}

	if manifest.Execution.WorkingDir != "app" {
		return result, fmt.Errorf(
			"replay: unsupported v4 working directory",
		)
	}

	if len(manifest.LocalModules) != 1 ||
		len(manifest.GoSelectedModules) != 1 {
		return result, fmt.Errorf(
			"replay: unsupported v4 local module graph",
		)
	}

	localModule := manifest.LocalModules[0].ModulePath

	selected := manifest.GoSelectedModules[0]
	if selected.Path != localModule ||
		selected.Version != "v0.0.0" ||
		selected.Replace == nil ||
		selected.Replace.Path != "../lib" ||
		selected.Replace.SnapshotPath != "lib" {
		return result, fmt.Errorf(
			"replay: inconsistent v4 local module selection",
		)
	}

	var mainGoMod []byte
	for _, file := range verified.Files {
		if file.Path == "app/go.mod" {
			mainGoMod = file.Data
			break
		}
	}

	if mainGoMod == nil {
		return result, fmt.Errorf(
			"replay: missing v4 main go.mod",
		)
	}

	parsed, err := modfile.Parse(
		"app/go.mod", mainGoMod, nil,
	)
	if err != nil {
		return result, fmt.Errorf(
			"replay: parse v4 main go.mod: %w", err,
		)
	}

	if parsed.Module == nil ||
		parsed.Module.Mod.Path == "" {
		return result, fmt.Errorf(
			"replay: missing v4 main module identity",
		)
	}

	mainModule := parsed.Module.Mod.Path

	uid := os.Geteuid()
	if uid == 0 {
		return result, fmt.Errorf(
			"replay: refuses to run Docker as root",
		)
	}

	containerUser := strconv.Itoa(uid) +
		":" + strconv.Itoa(os.Getegid())

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// Convert only verified V4 workspace content into the
	// Docker snapshot. The snapshot writer independently
	// rechecks file metadata and content hashes.
	var workspace bytes.Buffer

	if err := capsule.WriteWorkspaceSnapshotV4(
		&workspace, verified,
	); err != nil {
		return result, fmt.Errorf(
			"replay: prepare v4 workspace snapshot: %w",
			err,
		)
	}

	// Schema V4 does not capture external module artifacts.
	// Docker nevertheless requires a second valid TAR stream
	// for its existing two-volume snapshot interface.
	var modules bytes.Buffer

	moduleWriter := tar.NewWriter(&modules)
	if err := moduleWriter.Close(); err != nil {
		return result, fmt.Errorf(
			"replay: prepare empty v4 module snapshot: %w",
			err,
		)
	}

	var stderr boundedOutput

	code, err := docker.RunWithSDKRuntimeSnapshotExecReadersV4(
		ctx,
		bytes.NewReader(workspace.Bytes()),
		bytes.NewReader(modules.Bytes()),
		manifest.Execution.Argv,
		mainModule,
		localModule,
		options.MemoryBytes,
		containerUser,
		io.Discard,
		&stderr,
		&docker.RuntimeSpec{
			Kind:    manifest.Runtime.Kind,
			Version: manifest.Runtime.Version,
		},
	)
	if err != nil {
		return result, fmt.Errorf(
			"replay: v4 offline Docker execution: %w",
			err,
		)
	}

	outcome, err := compareFailureV3(
		manifest.Execution.ExitCode,
		manifest.Execution.Stderr,
		manifest.Execution.StderrTruncated,
		code,
		stderr.Bytes(),
		stderr.Truncated(),
	)
	if err != nil {
		return result, fmt.Errorf(
			"replay: compare v4 failure: %w",
			err,
		)
	}

	var diagnostics *StderrDiagnosticsV3

	if options.ShowStderr {
		diagnostics = newStderrDiagnosticsV3(
			manifest.Execution.Stderr,
			manifest.Execution.StderrTruncated,
			stderr.Bytes(),
			stderr.Truncated(),
		)
	}

	return Result{
		OriginalExitCode: manifest.Execution.ExitCode,
		ReplayExitCode:   code,
		Outcome:          outcome,
		Difference: DiagnoseFailureDifferenceV3(
			outcome,
			manifest.Execution.ExitCode,
			manifest.Execution.Stderr,
			code,
			stderr.Bytes(),
		),
		StderrDiagnostics: diagnostics,
	}, nil
}
