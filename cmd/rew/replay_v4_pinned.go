package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/rewtools/rew/internal/replay"
)

// runPinnedV4Replay does not open the capsule pathname.
// Classification and complete verification use the same
// caller-owned file descriptor.
func runPinnedV4Replay(
	ctx context.Context,
	f *os.File,
	options replay.Options,
) (replay.Result, error) {
	kind, err := inspectReplayV4KindFromFile(f)
	if err != nil {
		return replay.Result{}, fmt.Errorf(
			"replay: verify v4 capsule routing: %w", err,
		)
	}

	switch kind {
	case replayV4Local:
		return replay.RunV4WithOptionsFromFile(ctx, f, options)

	case replayV4Mixed:
		return replay.RunMixedV4WithOptionsFromFile(ctx, f, options)

	default:
		return replay.Result{}, fmt.Errorf(
			"replay: unsupported v4 capsule route",
		)
	}
}

// runPinnedV4ReplayPath opens a capsule exactly once.
// O_NOFOLLOW rejects symlinks at the final path component.
func runPinnedV4ReplayPath(
	ctx context.Context,
	path string,
	options replay.Options,
) (result replay.Result, resultErr error) {
	f, err := os.OpenFile(
		path, os.O_RDONLY|syscall.O_NOFOLLOW, 0,
	)
	if err != nil {
		return result, fmt.Errorf(
			"replay: verify v4 capsule routing: open v4 capsule: %w",
			err,
		)
	}
	defer func() {
		resultErr = errors.Join(resultErr, f.Close())
	}()

	return runPinnedV4Replay(ctx, f, options)
}
