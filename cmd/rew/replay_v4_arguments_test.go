package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestReplayV4ContextHasNoLegacyThirtySecondDeadline(t *testing.T) {
	ctx, cancel := replayContext(context.Background(), true)
	defer cancel()

	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		t.Fatal("V4 replay unexpectedly has a 30-second deadline")
	}
}

func TestReplayV4AcceptsSupportedArgumentShapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.rew")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "basic",
			args: []string{"--v4", "--allow-command", path},
		},
		{
			name: "memory",
			args: []string{
				"--v4", "--allow-command",
				"--memory", "512m", path,
			},
		},
		{
			name: "stderr",
			args: []string{
				"--v4", "--allow-command",
				"--show-stderr", path,
			},
		},
		{
			name: "memory and stderr",
			args: []string{
				"--v4", "--allow-command",
				"--memory", "512m",
				"--show-stderr", path,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code := runReplay(tc.args)

			// The missing capsule must reach V4 verification,
			// which produces exit 125, not CLI usage exit 2.
			if code != 125 {
				t.Fatalf(
					"exit=%d, want 125 (V4 verification)",
					code,
				)
			}
		})
	}
}

func TestReplayV4RejectsUnsupportedFlagOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.rew")

	code := runReplay([]string{
		"--v4",
		"--memory", "512m",
		"--allow-command",
		path,
	})

	if code != 2 {
		t.Fatalf("exit=%d, want usage error 2", code)
	}
}

func TestReplayV4ContextCanBeCancelled(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := replayContext(parent, true)
	defer cancel()

	cancelParent()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("V4 replay did not observe cancellation")
	}
}
