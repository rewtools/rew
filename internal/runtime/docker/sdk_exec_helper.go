package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

const maxExecOutputBytes = 1 << 20

type limitedExecBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *limitedExecBuffer) Write(p []byte) (int, error) {
	n := len(p)

	remaining := maxExecOutputBytes - b.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = b.Buffer.Write(p[:remaining])
	}

	if n > remaining {
		b.truncated = true
	}

	return n, nil
}

func executeInContainer(
	ctx context.Context,
	dockerClient *client.Client,
	containerID string,
	user string,
	argv []string,
) (int, []byte, []byte, error) {
	return executeInContainerAtDirectory(
		ctx, dockerClient, containerID, user,
		snapshotExecWorkingDirV3, argv,
	)
}

// executeInContainerAtDirectory executes inside an allowlisted
// snapshot workspace directory without changing the V3 default.
func executeInContainerAtDirectory(
	ctx context.Context,
	dockerClient *client.Client,
	containerID string,
	user string,
	workingDir string,
	argv []string,
) (int, []byte, []byte, error) {
	if err := validateSnapshotExecWorkingDirV4(workingDir); err != nil {
		return 0, nil, nil, err
	}
	if len(argv) == 0 {
		return 0, nil, nil, fmt.Errorf("docker SDK: empty exec argv")
	}

	if err := ctx.Err(); err != nil {
		return 0, nil, nil, fmt.Errorf(
			"docker SDK: exec context already canceled: %w",
			err,
		)
	}

	created, err := dockerClient.ExecCreate(
		ctx,
		containerID,
		client.ExecCreateOptions{
			User:         user,
			Privileged:   false,
			TTY:          false,
			AttachStdout: true,
			AttachStderr: true,
			WorkingDir:   workingDir,
			Cmd:          argv,
		},
	)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("docker SDK: exec create: %w", err)
	}

	attached, err := dockerClient.ExecAttach(
		ctx,
		created.ID,
		client.ExecAttachOptions{TTY: false},
	)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("docker SDK: exec attach: %w", err)
	}
	defer attached.Close()

	var stdout, stderr limitedExecBuffer

	readDone := make(chan error, 1)
	go func() {
		_, readErr := stdcopy.StdCopy(
			&stdout,
			&stderr,
			attached.Reader,
		)
		readDone <- readErr
	}()

	select {
	case readErr := <-readDone:
		if readErr != nil && readErr != io.EOF {
			return 0, stdout.Bytes(), stderr.Bytes(),
				fmt.Errorf("docker SDK: exec streams: %w", readErr)
		}

	case <-ctx.Done():
		attached.Close()

		select {
		case <-readDone:
		case <-time.After(2 * time.Second):
			// Docker connection did not terminate promptly.
			// Container cleanup is handled by the caller.
		}

		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf("docker SDK: exec canceled: %w", ctx.Err())
	}

	if err := ctx.Err(); err != nil {
		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf("docker SDK: exec canceled before inspect: %w", err)
	}

	inspected, err := dockerClient.ExecInspect(
		ctx,
		created.ID,
		client.ExecInspectOptions{},
	)
	if err != nil {
		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf("docker SDK: exec inspect: %w", err)
	}

	if inspected.Running {
		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf("docker SDK: exec still running after stream EOF")
	}

	if inspected.PID <= 0 {
		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf(
				"docker SDK: exec did not report a started process (pid=%d, exit=%d)",
				inspected.PID,
				inspected.ExitCode,
			)
	}

	if stdout.truncated || stderr.truncated {
		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf(
				"docker SDK: exec output exceeded %d bytes per stream",
				maxExecOutputBytes,
			)
	}

	if err := ctx.Err(); err != nil {
		return 0, stdout.Bytes(), stderr.Bytes(),
			fmt.Errorf("docker SDK: exec canceled after inspect: %w", err)
	}

	return inspected.ExitCode, stdout.Bytes(), stderr.Bytes(), nil
}
