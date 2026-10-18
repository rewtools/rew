package capsule

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreWorkspaceV4OfflineGoBuild(t *testing.T) {
	manifest := validLocalReplaceManifestV4()
	files := validWorkspaceWriterFilesV4()

	var archive bytes.Buffer
	if err := WriteWorkspaceV4(&archive, manifest, files); err != nil {
		t.Fatalf("write V4 archive: %v", err)
	}

	filename := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	verified, err := ReadVerifiedV4(filename)
	if err != nil {
		t.Fatalf("verify V4 archive: %v", err)
	}

	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(root, verified); err != nil {
		t.Fatalf("restore V4 workspace: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "build", "./...")
	command.Dir = filepath.Join(root, "app")

	// Isolate module discovery and disable network dependency fetching.
	// GOCACHE remains the Go toolchain's ordinary local cache.
	command.Env = append(os.Environ(),
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOWORK=off",
		"GOTOOLCHAIN=local",
		"CGO_ENABLED=0",
		"GOFLAGS=",
	)

	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("offline go build timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf(
			"offline go build failed: %v\n%s",
			err,
			output,
		)
	}
}
