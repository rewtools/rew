package replay

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rewtools/rew/internal/capsule"
)

func TestReplayV4LocalReplaceDockerE2E(t *testing.T) {
	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Skip("set REW_DOCKER_TEST=1 for Docker E2E")
	}

	files := []capsule.WorkspaceFile{
		{
			Path: "app/go.mod",
			Data: []byte(
				"module example.com/app\n\n" +
					"go 1.25\n\n" +
					"require example.com/local-lib v0.0.0\n\n" +
					"replace example.com/local-lib => ../lib\n",
			),
			Mode: 0600,
		},
		{
			Path: "app/main.go",
			Data: []byte(
				"package main\n\n" +
					"import _ \"example.com/local-lib\"\n\n" +
					"func main() {}\n",
			),
			Mode: 0600,
		},
		{
			Path: "lib/go.mod",
			Data: []byte(
				"module example.com/local-lib\n\ngo 1.25\n",
			),
			Mode: 0600,
		},
		{
			Path: "lib/lib.go",
			Data: []byte(
				"package lib\n\nfunc Broken( {\n",
			),
			Mode: 0600,
		},
	}

	placeholder := capsule.FileMetadata{
		Size:   10,
		SHA256: "sha256:" + strings.Repeat("a", 64),
	}

	manifest := capsule.ManifestV4{
		SchemaVersion: capsule.SchemaVersionV4,
		Execution: capsule.Execution{
			Argv:       []string{"go", "build", "./..."},
			WorkingDir: "app",
			ExitCode:   1,
			Stderr: "# example.com/local-lib\n" +
				"../lib/lib.go:3:14: syntax error: unexpected {, expected )\n",
		},
		Platform: capsule.Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
		Runtime: &capsule.Runtime{
			Kind:    "go",
			Version: capsule.SupportedGoVersion,
		},
		GoBuildEnv: &capsule.GoBuildEnvironment{
			GOOS:       "linux",
			GOARCH:     "amd64",
			CGOEnabled: "0",
		},
		Workspace: capsule.Workspace{
			Files: []capsule.FileMetadata{
				func() capsule.FileMetadata {
					f := placeholder
					f.Path = "app/go.mod"
					return f
				}(),
				func() capsule.FileMetadata {
					f := placeholder
					f.Path = "lib/go.mod"
					return f
				}(),
			},
		},
		GoSelectedModules: []capsule.GoModuleIdentityV4{
			{
				Path:    "example.com/local-lib",
				Version: "v0.0.0",
				Replace: &capsule.GoModuleReplacementV4{
					Path:         "../lib",
					SnapshotPath: "lib",
				},
			},
		},
		LocalModules: []capsule.LocalModuleV4{
			{
				ModulePath:   "example.com/local-lib",
				SnapshotPath: "lib",
				Files: []capsule.FileMetadata{
					func() capsule.FileMetadata {
						f := placeholder
						f.Path = "go.mod"
						return f
					}(),
				},
			},
		},
	}

	var archive bytes.Buffer
	if err := capsule.WriteWorkspaceV4(
		&archive, manifest, files,
	); err != nil {
		t.Fatalf("write V4 capsule: %v", err)
	}

	capsulePath := filepath.Join(t.TempDir(), "failure.rew")

	if err := os.WriteFile(
		capsulePath, archive.Bytes(), 0600,
	); err != nil {
		t.Fatal(err)
	}

	verified, err := capsule.ReadVerifiedV4(capsulePath)
	if err != nil {
		t.Fatalf("verify written V4 capsule: %v", err)
	}

	if verified.Manifest.SchemaVersion != capsule.SchemaVersionV4 {
		t.Fatal("unexpected V4 schema")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 6*time.Minute,
	)
	defer cancel()

	result, err := RunV4WithOptions(
		ctx,
		capsulePath,
		Options{
			AllowArbitraryCommand: true,
			ShowStderr:            true,
			MemoryBytes:           512 << 20,
		},
	)
	if err != nil {
		t.Fatalf("V4 replay failed: %v", err)
	}

	t.Logf("Original exit: %d", result.OriginalExitCode)
	t.Logf("Replay exit: %d", result.ReplayExitCode)
	t.Logf("Outcome: %s", result.Outcome)

	if result.OriginalExitCode != 1 {
		t.Fatalf(
			"original exit=%d, want 1",
			result.OriginalExitCode,
		)
	}
	if result.ReplayExitCode != 1 {
		t.Fatalf(
			"replay exit=%d, want 1",
			result.ReplayExitCode,
		)
	}
	if result.Outcome != OutcomeReproduced {
		t.Fatalf(
			"outcome=%s, want reproduced",
			result.Outcome,
		)
	}
}
