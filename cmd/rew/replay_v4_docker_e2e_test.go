package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestReplayV4CLIDockerE2E(t *testing.T) {
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
			Data: []byte("package lib\n\nfunc Broken( {\n"),
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
				{
					Path:   "app/go.mod",
					Size:   placeholder.Size,
					SHA256: placeholder.SHA256,
				},
				{
					Path:   "lib/go.mod",
					Size:   placeholder.Size,
					SHA256: placeholder.SHA256,
				},
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
					{
						Path:   "go.mod",
						Size:   placeholder.Size,
						SHA256: placeholder.SHA256,
					},
				},
			},
		},
	}

	var archive bytes.Buffer
	if err := capsule.WriteWorkspaceV4(
		&archive, manifest, files,
	); err != nil {
		t.Fatalf("create verified V4 fixture: %v", err)
	}

	path := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := capsule.ReadVerifiedV4(path); err != nil {
		t.Fatalf("verify V4 fixture: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "cli-output.txt")
	outputFile, err := os.OpenFile(
		outputPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer outputFile.Close()

	originalStdout := os.Stdout
	os.Stdout = outputFile

	code := func() int {
		defer func() { os.Stdout = originalStdout }()
		return runReplay([]string{
			"--v4",
			"--allow-command",
			"--memory", "512m",
			"--show-stderr",
			path,
		})
	}()

	if _, err := outputFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(outputBytes)
	t.Logf("V4 CLI output:\n%s", output)

	if code != 0 {
		t.Fatalf("CLI exit code=%d, want 0", code)
	}

	for _, expected := range []string{
		"Original exit code: 1",
		"Replay exit code:",
		"Outcome:",
		"reproduced",
		"Replay memory:",
		"512 MiB",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf(
				"CLI output missing %q:\n%s",
				expected, output,
			)
		}
	}

	if !strings.Contains(
		output, "Replay exit code:   1",
	) && !strings.Contains(
		output, "Replay exit code: 1",
	) {
		t.Fatalf("unexpected replay exit code:\n%s", output)
	}
}
