package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV3WithoutGoSum(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOTOOLCHAIN", "local")

	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	files := map[string]string{
		"go.mod":  "module example.com/no-sum\n\ngo 1.26.0\n",
		"main.go": "package main\nfunc main() { missingSymbol() }\n",
	}

	for name, data := range files {
		if err := os.WriteFile(
			filepath.Join(workspace, name),
			[]byte(data),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "main.go"},
		output,
	)
	if code != 1 {
		t.Fatalf("capture exit code=%d, want 1", code)
	}

	if _, err := os.Lstat(filepath.Join(workspace, "go.sum")); !os.IsNotExist(err) {
		t.Fatalf("capture created or changed go.sum: %v", err)
	}

	verified, err := capsule.ReadVerifiedV3(output)
	if err != nil {
		t.Fatalf("verify capsule: %v", err)
	}

	if len(verified.Manifest.GoModules) != 0 {
		t.Fatalf("expected zero external modules, got %d",
			len(verified.Manifest.GoModules))
	}

	found := false
	for _, file := range verified.Files {
		if file.Path == "go.sum" {
			found = true
			if len(file.Data) != 0 {
				t.Fatal("virtual go.sum must be empty")
			}
		}
	}
	if !found {
		t.Fatal("virtual go.sum missing from capsule")
	}
}

func TestCaptureV3RejectsExistingUnincludedGoSum(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOTOOLCHAIN", "local")

	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	files := map[string]string{
		"go.mod":  "module example.com/existing-sum\n\ngo 1.26.0\n",
		"go.sum":  "",
		"main.go": "package main\nfunc main() { missingSymbol() }\n",
	}

	for name, data := range files {
		if err := os.WriteFile(
			filepath.Join(workspace, name),
			[]byte(data),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "main.go"},
		output,
	)

	if code != 125 {
		t.Fatalf("capture exit code=%d, want 125", code)
	}

	requireNoCaptureV3Archive(t, output)
}

func TestCaptureV3RejectsDependencyWithoutGoSum(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOTOOLCHAIN", "local")

	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	files := map[string]string{
		"go.mod": "module example.com/missing-dependency-sum\n\ngo 1.26.0\n\n" +
			"require example.com/missing-dependency v1.0.0\n",
		"main.go": "package main\nfunc main() { missingSymbol() }\n",
	}

	for name, data := range files {
		if err := os.WriteFile(
			filepath.Join(workspace, name),
			[]byte(data),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"go.mod", "main.go"},
		output,
	)

	if code != 125 {
		t.Fatalf("capture exit code=%d, want 125", code)
	}

	requireNoCaptureV3Archive(t, output)
}

func TestCaptureV3RejectsCachedDependencyWithoutGoSum(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	fixture := makeCaptureV3Fixture(t)

	goSumPath := filepath.Join(fixture.workspace, "go.sum")
	if err := os.Remove(goSumPath); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(goSumPath); !os.IsNotExist(err) {
		t.Fatalf("expected go.sum to be absent: %v", err)
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		fixture.workspace,
		[]string{"go.mod", "main.go"},
		fixture.output,
	)

	if code != 125 {
		t.Fatalf("capture exit code=%d, want 125", code)
	}

	requireNoCaptureV3Archive(t, fixture.output)
}
