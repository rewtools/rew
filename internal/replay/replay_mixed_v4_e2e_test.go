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

func TestRunMixedV4FullCapsuleDockerE2E(t *testing.T) {
	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Skip("set REW_DOCKER_TEST=1 for Docker E2E")
	}
	if os.Geteuid() == 0 {
		t.Skip("mixed replay refuses root execution")
	}

	verified := mixedSnapshotFixtureV4(t)

	const originalStderr = "# example.com/local-lib\n" +
		"../lib/lib.go:2:14: syntax error: unexpected {, expected )\n"

	manifest := verified.Manifest
	manifest.Execution.Argv = []string{"go", "build", "./..."}
	manifest.Execution.ExitCode = 1
	manifest.Execution.Stderr = originalStderr
	manifest.Execution.StderrTruncated = false

	files := append([]capsule.WorkspaceFile(nil), verified.Files...)

	foundMain := false
	foundLocal := false

	for i := range files {
		switch files[i].Path {
		case "app/main.go":
			files[i].Data = []byte(
				"package main\n" +
					"import _ \"example.com/external\"\n" +
					"import _ \"example.com/local-lib\"\n" +
					"func main() {}\n",
			)
			foundMain = true

		case "lib/lib.go":
			files[i].Data = []byte(
				"package local\nfunc Broken( {\n",
			)
			foundLocal = true
		}
	}

	if !foundMain || !foundLocal {
		t.Fatal("mixed fixture missing required source files")
	}

	var archive bytes.Buffer

	if err := capsule.WriteWorkspaceFromStoreV4(
		&archive,
		manifest,
		files,
		verified.Store,
	); err != nil {
		t.Fatalf("create failing mixed capsule: %v", err)
	}

	capsulePath := filepath.Join(t.TempDir(), "failure.rew")

	if err := os.WriteFile(
		capsulePath,
		archive.Bytes(),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	reader, err := capsule.ReadVerifiedToStoreV4(capsulePath)
	if err != nil {
		t.Fatalf("verify written mixed capsule: %v", err)
	}

	if reader.Manifest.Execution.Stderr != originalStderr {
		t.Fatal("verified capsule lost original stderr")
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("close verification reader: %v", err)
	}

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	assertNoTempResources := func(t *testing.T) {
		t.Helper()
		entries, err := os.ReadDir(tempRoot)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			t.Fatalf(
				"mixed replay leaked temporary resources: %v",
				names,
			)
		}
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		6*time.Minute,
	)
	defer cancel()

	t.Run("explicit execution opt-in required", func(t *testing.T) {
		_, err := RunMixedV4WithOptions(
			ctx,
			capsulePath,
			Options{
				MemoryBytes: 512 << 20,
			},
		)
		if err == nil {
			t.Fatal("mixed replay executed without opt-in")
		}
		if !strings.Contains(
			err.Error(),
			"explicit opt-in",
		) {
			t.Fatalf("unexpected opt-in error: %v", err)
		}
	})

	t.Run("verified capsule replays original failure", func(t *testing.T) {
		result, err := RunMixedV4WithOptions(
			ctx,
			capsulePath,
			Options{
				AllowArbitraryCommand: true,
				ShowStderr:            true,
				MemoryBytes:           512 << 20,
			},
		)
		if err != nil {
			t.Fatalf("mixed capsule replay failed: %v", err)
		}

		t.Logf(
			"Mixed V4 result: outcome=%s original=%d replay=%d",
			result.Outcome,
			result.OriginalExitCode,
			result.ReplayExitCode,
		)

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
				"outcome=%q, want %q",
				result.Outcome,
				OutcomeReproduced,
			)
		}
		if result.StderrDiagnostics == nil {
			t.Fatal("missing opted-in stderr diagnostics")
		}
		assertNoTempResources(t)
	})

	t.Run("Docker initialization failure cleans resources", func(t *testing.T) {
		t.Setenv(
			"DOCKER_HOST",
			"unix:///nonexistent/rew-mixed-v4-test/docker.sock",
		)

		_, err := RunMixedV4WithOptions(
			ctx,
			capsulePath,
			Options{
				AllowArbitraryCommand: true,
				MemoryBytes:           512 << 20,
			},
		)
		if err == nil {
			t.Fatal("expected Docker initialization failure")
		}
		if !strings.Contains(err.Error(), "Docker") &&
			!strings.Contains(err.Error(), "docker") {
			t.Fatalf("unexpected failure stage: %v", err)
		}

		assertNoTempResources(t)
		t.Logf("Docker failure cleaned temporary resources: %v", err)
	})
}
