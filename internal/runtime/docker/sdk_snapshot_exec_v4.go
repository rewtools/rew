package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"golang.org/x/mod/module"
)

// RunWithSDKRuntimeSnapshotExecReadersV4 executes a schema-v4
// local-module Go build in an isolated Docker snapshot environment.
//
// Both TAR readers must originate from verified capsule data.
// This function does not accept host bind mounts.
func RunWithSDKRuntimeSnapshotExecReadersV4(
	ctx context.Context,
	workspaceTAR io.Reader,
	moduleTAR io.Reader,
	argv []string,
	mainModule string,
	localModule string,
	memoryBytes int64,
	user string,
	stdout, stderr io.Writer,
	runtime *RuntimeSpec,
) (int, error) {
	return runWithSDKRuntimeSnapshotExecReadersV4(
		ctx, workspaceTAR, moduleTAR, argv,
		mainModule, localModule, "", "",
		memoryBytes, user, stdout, stderr, runtime,
	)
}

// RunWithSDKRuntimeSnapshotExecReadersMixedV4 replays a mixed V4
// graph with one local replacement and one external Go module.
// Both TAR readers must originate from verified capsule data.
func RunWithSDKRuntimeSnapshotExecReadersMixedV4(
	ctx context.Context,
	workspaceTAR io.Reader,
	moduleTAR io.Reader,
	argv []string,
	mainModule string,
	localModule string,
	externalModule string,
	externalVersion string,
	memoryBytes int64,
	user string,
	stdout, stderr io.Writer,
	runtime *RuntimeSpec,
) (int, error) {
	if err := module.Check(externalModule, externalVersion); err != nil {
		return 0, fmt.Errorf(
			"docker SDK: invalid mixed v4 external module: %w", err,
		)
	}
	if externalModule == mainModule || externalModule == localModule {
		return 0, fmt.Errorf(
			"docker SDK: mixed v4 external module overlaps local graph",
		)
	}
	return runWithSDKRuntimeSnapshotExecReadersV4(
		ctx, workspaceTAR, moduleTAR, argv,
		mainModule, localModule, externalModule, externalVersion,
		memoryBytes, user, stdout, stderr, runtime,
	)
}

func runWithSDKRuntimeSnapshotExecReadersV4(

	ctx context.Context,
	workspaceTAR io.Reader,
	moduleTAR io.Reader,
	argv []string,
	mainModule string,
	localModule string,
	externalModule string,
	externalVersion string,
	memoryBytes int64,
	user string,
	stdout, stderr io.Writer,
	runtime *RuntimeSpec,
) (exitCode int, runErr error) {
	if err := module.CheckPath(mainModule); err != nil {
		return 0, fmt.Errorf("docker SDK: invalid main module: %w", err)
	}
	if err := module.CheckPath(localModule); err != nil {
		return 0, fmt.Errorf("docker SDK: invalid local module: %w", err)
	}
	if mainModule == localModule {
		return 0, fmt.Errorf("docker SDK: identical main and local modules")
	}
	if len(argv) == 0 {
		return 0, fmt.Errorf("docker SDK: empty v4 command")
	}
	if workspaceTAR == nil || moduleTAR == nil {
		return 0, fmt.Errorf("docker SDK: nil v4 snapshot reader")
	}
	if err := validateReplayMemoryV3(memoryBytes); err != nil {
		return 0, err
	}
	if runtime == nil || runtime.Kind != "go" ||
		!isSupportedGoRuntime(runtime) {
		return 0, fmt.Errorf("docker SDK: unsupported v4 Go runtime")
	}
	if err := validateSnapshotExecWorkingDirV4(
		snapshotExecWorkingDirV4,
	); err != nil {
		return 0, err
	}

	parts := strings.Split(user, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("docker SDK: invalid v4 UID:GID")
	}
	uid, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil || uid == 0 {
		return 0, fmt.Errorf("docker SDK: invalid v4 UID")
	}
	if _, err := strconv.ParseUint(parts[1], 10, 32); err != nil {
		return 0, fmt.Errorf("docker SDK: invalid v4 GID")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	image, err := selectReplayImage(runtime)
	if err != nil {
		return 0, err
	}

	dockerClient, err := NewSDKClient()
	if err != nil {
		return 0, err
	}
	defer dockerClient.Close()

	prepareCtx, cancelPrepare := context.WithTimeout(
		ctx, 120*time.Second,
	)
	prepareErr := ensureReplayImage(
		prepareCtx, dockerClient, image,
	)
	cancelPrepare()
	if prepareErr != nil {
		return 0, fmt.Errorf(
			"docker SDK: prepare v4 image: %w", prepareErr,
		)
	}

	execCtx, cancelExec := context.WithTimeout(
		ctx, snapshotGoExecutionTimeoutV3,
	)
	defer cancelExec()

	volumes, err := CreateSnapshotVolumesFromReaders(
		execCtx, dockerClient, image,
		workspaceTAR, moduleTAR,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"docker SDK: prepare v4 snapshot volumes: %w", err,
		)
	}
	defer func() {
		if cleanupErr := volumes.Close(); cleanupErr != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf(
					"docker SDK: cleanup v4 volumes: %w",
					cleanupErr,
				),
			)
		}
	}()

	options := newSnapshotExecutionOptions(
		image, user, volumes,
	)
	options.Config.WorkingDir = snapshotExecWorkingDirV4

	if err := applyReplayMemoryV3(
		&options, memoryBytes,
	); err != nil {
		return 0, err
	}

	created, err := createSnapshotExecutionContainer(
		execCtx, dockerClient, options,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"docker SDK: create v4 container: %w", err,
		)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()

		_, cleanupErr := dockerClient.ContainerRemove(
			cleanupCtx, created.ID,
			client.ContainerRemoveOptions{
				Force: true, RemoveVolumes: true,
			},
		)
		if cleanupErr != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf(
					"docker SDK: cleanup v4 container: %w",
					cleanupErr,
				),
			)
		}
	}()

	if _, err := dockerClient.ContainerStart(
		execCtx, created.ID, client.ContainerStartOptions{},
	); err != nil {
		return 0, fmt.Errorf(
			"docker SDK: start v4 container: %w", err,
		)
	}

	// Copy the module cache, preserving the existing offline
	// execution model. The V4 profile has no remote modules.
	prepareCode, _, prepareStderr, err :=
		executeInContainerAtDirectory(
			execCtx, dockerClient, created.ID, user,
			snapshotExecWorkingDirV4,
			[]string{
				"/bin/sh", "-ec",
				"cp -R /module-cache-source/. /gomodcache/",
			},
		)
	if err != nil {
		return 0, fmt.Errorf(
			"docker SDK: prepare v4 module cache: %w", err,
		)
	}
	if prepareCode != 0 {
		return 0, fmt.Errorf(
			"docker SDK: v4 module cache preparation failed (%d): %s",
			prepareCode, string(prepareStderr),
		)
	}

	if externalModule != "" {
		downloadCode, _, downloadStderr, downloadErr :=
			executeInContainerAtDirectory(
				execCtx, dockerClient, created.ID, user,
				snapshotExecWorkingDirV4,
				[]string{
					"go", "mod", "download",
					externalModule + "@" + externalVersion,
				},
			)
		if downloadErr != nil {
			return 0, fmt.Errorf(
				"docker SDK: mixed v4 offline module preparation: %w",
				downloadErr,
			)
		}
		if downloadCode != 0 {
			return 0, fmt.Errorf(
				"docker SDK: mixed v4 offline module preparation failed (%d): %s",
				downloadCode, string(downloadStderr),
			)
		}
	}

	graphCode, graphOutput, graphStderr, err :=
		executeInContainerAtDirectory(
			execCtx, dockerClient, created.ID, user,
			snapshotExecWorkingDirV4,
			[]string{"go", "list", "-m", "-json", "all"},
		)
	if err != nil {
		return 0, fmt.Errorf(
			"docker SDK: v4 graph command: %w", err,
		)
	}
	if graphCode != 0 {
		return 0, fmt.Errorf(
			"docker SDK: v4 module graph failed (%d): %s",
			graphCode, string(graphStderr),
		)
	}
	var graphErr error
	if externalModule != "" {
		graphErr = verifyDockerGoModuleGraphMixedV4(
			graphOutput,
			mainModule,
			localModule,
			"/workspace/lib",
			externalModule,
			externalVersion,
		)
	} else {
		graphErr = verifyDockerGoModuleGraphV4(
			graphOutput,
			mainModule,
			localModule,
			"/workspace/lib",
		)
	}
	if graphErr != nil {
		return 0, fmt.Errorf(
			"docker SDK: v4 module graph mismatch: %w", graphErr,
		)
	}

	code, out, errOut, err := executeInContainerAtDirectory(
		execCtx, dockerClient, created.ID, user,
		snapshotExecWorkingDirV4, argv,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"docker SDK: v4 execution: %w", err,
		)
	}
	if stdout != nil {
		if _, err := stdout.Write(out); err != nil {
			return 0, fmt.Errorf("docker SDK: v4 stdout: %w", err)
		}
	}
	if stderr != nil {
		if _, err := stderr.Write(errOut); err != nil {
			return 0, fmt.Errorf("docker SDK: v4 stderr: %w", err)
		}
	}

	return code, nil
}
