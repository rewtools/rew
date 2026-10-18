package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/replay"
)

func TestCaptureReplayV3ZeroDependenciesEndToEnd(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Skip("Docker integration test disabled")
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("requires macOS or Linux")
	}

	if os.Geteuid() == 0 {
		t.Skip("Docker replay requires non-root execution")
	}

	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")

	workspace := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/rew-zero-e2e\n\ngo 1.26.6\n",
		"go.sum": "",
		"main.go": `package main

func main() {
	undefinedREWSymbol()
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(
			filepath.Join(workspace, name),
			[]byte(content),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	capsulePath := filepath.Join(t.TempDir(), "failure.bfc")

	captureExit := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "go.sum", "main.go"},
		capsulePath,
	)

	if captureExit != 1 {
		t.Fatalf("capture exit=%d, want 1", captureExit)
	}

	verified, err := capsule.ReadVerifiedV3(capsulePath)
	if err != nil {
		t.Fatalf("verify v3 capsule: %v", err)
	}

	if len(verified.Manifest.GoModules) != 0 {
		t.Fatalf(
			"expected zero Go modules, got %d",
			len(verified.Manifest.GoModules),
		)
	}

	if len(verified.Artifacts) != 0 {
		t.Fatalf(
			"expected zero Go artifacts, got %d",
			len(verified.Artifacts),
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		120*time.Second,
	)
	defer cancel()

	result, err := replay.RunV3WithOptions(
		ctx,
		capsulePath,
		replay.Options{AllowArbitraryCommand: true},
	)
	if err != nil {
		t.Fatalf("offline Docker replay: %v", err)
	}

	if result.Outcome != replay.OutcomeReproduced {
		t.Fatalf(
			"outcome=%q, original=%d, replay=%d",
			result.Outcome,
			result.OriginalExitCode,
			result.ReplayExitCode,
		)
	}

	if result.OriginalExitCode != 1 || result.ReplayExitCode != 1 {
		t.Fatalf(
			"unexpected exit codes: original=%d replay=%d",
			result.OriginalExitCode,
			result.ReplayExitCode,
		)
	}

	t.Log("schema v3 zero-dependency offline replay reproduced")
}
