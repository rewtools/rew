package replay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/runtime/docker"
	"golang.org/x/mod/modfile"
)

// RunMixedV4WithOptions is an experimental mixed-dependency replay
// entry point. It supports exactly one local replacement and one
// external Go module. It does not change the existing V4 replay API.
func RunMixedV4WithOptions(
	ctx context.Context,
	capsulePath string,
	options Options,
) (result Result, resultErr error) {
	if options.MemoryBytes != 0 {
		switch options.MemoryBytes {
		case 256 << 20, 512 << 20, 1 << 30, 2 << 30:
		default:
			return result, fmt.Errorf(
				"replay: invalid mixed v4 memory limit: %d bytes",
				options.MemoryBytes,
			)
		}
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	verified, err := capsule.ReadVerifiedToStoreV4(capsulePath)
	if err != nil {
		return result, fmt.Errorf(
			"replay: verify mixed v4 capsule: %w", err,
		)
	}
	defer func() {
		resultErr = errors.Join(resultErr, verified.Close())
	}()

	manifest := verified.Manifest

	if manifest.Platform.OS != "linux" ||
		manifest.Platform.Architecture != "amd64" {
		return result, fmt.Errorf(
			"replay: unsupported mixed v4 platform %s/%s",
			manifest.Platform.OS,
			manifest.Platform.Architecture,
		)
	}

	if !options.AllowArbitraryCommand {
		return result, fmt.Errorf(
			"replay: mixed v4 command execution requires explicit opt-in",
		)
	}

	if manifest.Runtime == nil ||
		manifest.Runtime.Kind != "go" ||
		!capsule.IsSupportedGoVersion(manifest.Runtime.Version) {
		return result, fmt.Errorf(
			"replay: unsupported mixed v4 Go runtime",
		)
	}

	if manifest.Execution.WorkingDir != "app" {
		return result, fmt.Errorf(
			"replay: unsupported mixed v4 working directory",
		)
	}

	if len(manifest.LocalModules) != 1 ||
		len(manifest.GoSelectedModules) != 2 {
		return result, fmt.Errorf(
			"replay: unsupported mixed v4 module graph",
		)
	}

	localModule := manifest.LocalModules[0].ModulePath

	var externalModule, externalVersion string
	foundLocal := false

	for _, selected := range manifest.GoSelectedModules {
		if selected.Path == localModule {
			if foundLocal ||
				selected.Version != "v0.0.0" ||
				selected.Replace == nil ||
				selected.Replace.Path != "../lib" ||
				selected.Replace.SnapshotPath != "lib" {
				return result, fmt.Errorf(
					"replay: invalid mixed v4 local module selection",
				)
			}
			foundLocal = true
			continue
		}

		if externalModule != "" ||
			selected.Replace != nil ||
			selected.Path == "" ||
			selected.Version == "" {
			return result, fmt.Errorf(
				"replay: invalid mixed v4 external module selection",
			)
		}

		externalModule = selected.Path
		externalVersion = selected.Version
	}

	if !foundLocal || externalModule == "" {
		return result, fmt.Errorf(
			"replay: incomplete mixed v4 module selection",
		)
	}

	var mainGoMod []byte
	for _, file := range verified.Files {
		if file.Path == "app/go.mod" {
			mainGoMod = file.Data
			break
		}
	}
	if len(mainGoMod) == 0 {
		return result, fmt.Errorf(
			"replay: missing mixed v4 main go.mod",
		)
	}

	parsed, err := modfile.Parse(
		"app/go.mod", mainGoMod, nil,
	)
	if err != nil {
		return result, fmt.Errorf(
			"replay: parse mixed v4 main go.mod: %w", err,
		)
	}
	if parsed.Module == nil ||
		parsed.Module.Mod.Path == "" {
		return result, fmt.Errorf(
			"replay: missing mixed v4 main module identity",
		)
	}
	mainModule := parsed.Module.Mod.Path

	if mainModule == localModule ||
		mainModule == externalModule {
		return result, fmt.Errorf(
			"replay: overlapping mixed v4 module identities",
		)
	}

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

	snapshots, err := prepareSnapshotFilesFromStoreV4(verified)
	if err != nil {
		return result, fmt.Errorf(
			"replay: prepare mixed v4 snapshots: %w", err,
		)
	}
	defer func() {
		resultErr = errors.Join(resultErr, snapshots.Close())
	}()

	var stderr boundedOutput

	// snapshotFilesV3 owns both files. Pass only their Read
	// capability: Docker must not close these underlying descriptors.
	// snapshots.Close remains responsible for releasing both files
	// and removing the temporary directory.
	workspaceReader := struct{ io.Reader }{snapshots.workspace}
	moduleReader := struct{ io.Reader }{snapshots.modules}

	code, err := docker.RunWithSDKRuntimeSnapshotExecReadersMixedV4(
		ctx,
		workspaceReader,
		moduleReader,
		manifest.Execution.Argv,
		mainModule,
		localModule,
		externalModule,
		externalVersion,
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
			"replay: mixed v4 offline Docker execution: %w",
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
			"replay: compare mixed v4 failure: %w", err,
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
