package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func makeCaptureV3TestWorkspace(t *testing.T) string {
	t.Helper()

	workspace := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/rew-capture-v3-test\n\n" +
			"go 1.26.0\n\n" +
			"require example.com/rew/fixture v1.0.0\n",
		"go.sum":  "example.com/rew/fixture v1.0.0 h1:invalid\n",
		"main.go": "package main\n\nfunc main() { missingSymbol() }\n",
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

	return workspace
}

func requireNoCaptureV3Archive(t *testing.T, path string) {
	t.Helper()

	_, err := os.Lstat(path)
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected capsule at %s: stat err=%v", path, err)
	}
}

func TestCaptureV3SuccessDoesNotCreateArchive(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	if err := os.WriteFile(
		filepath.Join(workspace, "go.mod"),
		[]byte("module example.com/capture-success\n\ngo 1.26.0\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(workspace, "fixture.go"),
		[]byte("package fixture\n\nfunc Value() int { return 42 }\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "fixture.go"},
		output,
	)

	if code != 0 {
		t.Fatalf("exit code=%d, want 0", code)
	}

	requireNoCaptureV3Archive(t, output)
}

func TestCaptureV3RejectsMissingGoSum(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	workspace := makeCaptureV3TestWorkspace(t)
	output := filepath.Join(t.TempDir(), "failure.bfc")

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "main.go"},
		output,
	)

	if code != 125 {
		t.Fatalf("exit code=%d, want 125", code)
	}

	requireNoCaptureV3Archive(t, output)
}

func TestCaptureV3RejectsUnsupportedRuntime(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	code := captureCommandV3(
		[]string{"/bin/sh", "-c", "exit 23"},
		workspace,
		[]string{"go.mod"},
		output,
	)

	if code != 125 {
		t.Fatalf("exit code=%d, want 125", code)
	}

	requireNoCaptureV3Archive(t, output)
}

func TestCaptureV3RejectsInvalidArguments(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	tests := []struct {
		name     string
		argv     []string
		includes []string
	}{
		{"missing argv", nil, []string{"go.mod"}},
		{"missing includes", []string{"go", "build"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := captureCommandV3(
				tt.argv,
				workspace,
				tt.includes,
				output,
			)
			if code != 2 {
				t.Fatalf("exit code=%d, want 2", code)
			}
			requireNoCaptureV3Archive(t, output)
		})
	}
}

func TestCaptureV3UnsupportedGoModDoesNotWriteArchive(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	workspace := makeCaptureV3TestWorkspace(t)
	output := filepath.Join(t.TempDir(), "failure.bfc")

	goMod := []byte(
		"module example.com/rew-capture-v3-test\n\n" +
			"go 1.26.0\n\n" +
			"require example.com/rew/fixture v1.0.0\n" +
			"replace example.com/rew/fixture => ../local\n",
	)

	if err := os.WriteFile(
		filepath.Join(workspace, "go.mod"),
		goMod,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "go.sum", "main.go"},
		output,
	)

	if code != 125 {
		t.Fatalf("exit code=%d, want 125", code)
	}

	requireNoCaptureV3Archive(t, output)
}

// These checks exercise the existing CLI argument routing.
// They must return usage errors before any build or Docker action.
func TestCaptureV3CLIInvalidArguments(t *testing.T) {
	cases := [][]string{
		{"capture", "--v3"},
		{"capture", "--v3", "--include"},
		{"capture", "--v3", "--include", "go.mod"},
		{"capture", "--v3", "--", "go", "build"},
		{"capture", "--v3", "--include", "go.mod", "--"},
	}

	for _, args := range cases {
		if code := run(args); code != 2 {
			t.Fatalf("run(%q) exit=%d, want 2", args, code)
		}
	}
}

// Ensure the Go toolchain is available at the exact version required
// by the v3 manifest contract.
func TestCaptureV3ToolchainVersion(t *testing.T) {
	output, err := exec.Command("go", "version").Output()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(
		output,
		[]byte("go"+capsule.SupportedGoVersion),
	) {
		t.Skipf("unsupported local toolchain: %s", output)
	}
}
