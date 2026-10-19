package replay

import (
	"context"
	"strings"
	"testing"
)

func TestRunMixedV4RejectsInvalidMemoryBeforeCapsuleRead(t *testing.T) {
	_, err := RunMixedV4WithOptions(
		context.Background(),
		"/nonexistent/rew-mixed-v4.rew",
		Options{
			MemoryBytes: 12345,
		},
	)
	if err == nil {
		t.Fatal("invalid replay memory limit accepted")
	}
	if !strings.Contains(err.Error(), "invalid") ||
		!strings.Contains(err.Error(), "memory") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRunMixedV4RejectsMissingCapsule(t *testing.T) {
	_, err := RunMixedV4WithOptions(
		context.Background(),
		"/nonexistent/rew-mixed-v4.rew",
		Options{},
	)
	if err == nil {
		t.Fatal("missing mixed V4 capsule accepted")
	}
}
