package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV4ToDockerReplayEndToEnd(t *testing.T) {
	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Skip("set REW_DOCKER_TEST=1 for real Docker E2E")
	}

	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{app, lib} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(
			name, []byte(content), 0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"),
		"module example.com/app\n\n"+
			"go 1.25\n\n"+
			"require example.com/local-lib v0.0.0\n\n"+
			"replace example.com/local-lib => ../lib\n",
	)

	write(filepath.Join(app, "main.go"),
		"package main\n\n"+
			"import _ \"example.com/local-lib\"\n\n"+
			"func main() {}\n",
	)

	write(filepath.Join(lib, "go.mod"),
		"module example.com/local-lib\n\ngo 1.25\n",
	)

	write(filepath.Join(lib, "lib.go"),
		"package lib\n\nfunc Broken( {\n",
	)

	output := filepath.Join(parent, "failure.rew")

	t.Log("Running real V4 capture")

	captureCode := captureCommandV4(
		[]string{"go", "build", "./..."},
		app,
		output,
	)

	if captureCode != 1 {
		t.Fatalf(
			"capture exit=%d, want original build exit 1",
			captureCode,
		)
	}

	verified, err := capsule.ReadVerifiedV4(output)
	if err != nil {
		t.Fatalf("verify captured capsule: %v", err)
	}

	if verified.Manifest.Execution.ExitCode != 1 {
		t.Fatalf(
			"recorded exit=%d, want 1",
			verified.Manifest.Execution.ExitCode,
		)
	}

	if len(verified.Manifest.LocalModules) != 1 {
		t.Fatal("expected exactly one captured local module")
	}

	found := map[string]bool{}
	for _, file := range verified.Files {
		found[file.Path] = true
	}

	for _, required := range []string{
		"app/go.mod",
		"app/main.go",
		"lib/go.mod",
		"lib/lib.go",
	} {
		if !found[required] {
			t.Fatalf(
				"real capture omitted %s",
				required,
			)
		}
	}

	t.Log("Running actual V4 CLI replay through Docker")

	// runReplay writes its summary to os.Stdout.
	// Capture it in a private temporary file.
	stdoutPath := filepath.Join(parent, "replay-stdout.txt")

	stdoutFile, err := os.OpenFile(
		stdoutPath,
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutFile.Close()

	originalStdout := os.Stdout

	replayCode := func() int {
		os.Stdout = stdoutFile
		defer func() {
			os.Stdout = originalStdout
		}()

		return runReplay([]string{
			"--v4",
			"--allow-command",
			"--memory", "512m",
			"--show-stderr",
			output,
		})
	}()

	stdoutBytes, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}

	summary := string(stdoutBytes)
	t.Logf("Real capture to replay output:\n%s", summary)

	if replayCode != 0 {
		t.Fatalf(
			"CLI replay exit=%d, want 0\n%s",
			replayCode, summary,
		)
	}

	for _, required := range []string{
		"Original exit code: 1",
		"Replay exit code:   1",
		"Outcome:            reproduced",
		"=== Original stderr ===",
		"=== Replay stderr ===",
		"../lib/lib.go:3:14: syntax error:",
		"Replay memory:      512 MiB",
	} {
		if !strings.Contains(summary, required) {
			t.Fatalf(
				"missing %q in replay output:\n%s",
				required, summary,
			)
		}
	}

	t.Log("REAL CAPTURE -> VERIFIED V4 -> DOCKER REPLAY: REPRODUCED")
}
