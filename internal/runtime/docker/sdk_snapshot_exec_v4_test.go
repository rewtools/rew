package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSnapshotExecutorV4RejectsInvalidLocalModule(t *testing.T) {
	var stdout, stderr bytes.Buffer

	_, err := RunWithSDKRuntimeSnapshotExecReadersV4(
		context.Background(),
		bytes.NewReader([]byte("workspace")),
		bytes.NewReader([]byte("modules")),
		[]string{"go", "build", "./..."},
		"example.com/app",
		"../lib",
		256<<20,
		"10001:10001",
		&stdout,
		&stderr,
		&RuntimeSpec{
			Kind:    "go",
			Version: "1.26.6",
		},
	)

	if err == nil {
		t.Fatal("invalid V4 local module accepted")
	}

	if !strings.Contains(err.Error(), "local module") {
		t.Fatalf("unexpected error: %v", err)
	}
}
