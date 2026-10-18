package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setupPortableCaptureV3Test(t *testing.T) {
	t.Helper()

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported test host")
	}

	if runtime.GOOS == "darwin" {
		t.Setenv("GOOS", "darwin")
		t.Setenv("GOARCH", "arm64")
	} else {
		t.Setenv("GOOS", "linux")
		t.Setenv("GOARCH", "amd64")
	}

	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
}

func TestCaptureV3PreflightRejectsCGO(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	setupPortableCaptureV3Test(t)

	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	t.Setenv("CGO_ENABLED", "1")

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"missing-file.go"},
		output,
	)

	if code != 125 {
		t.Fatalf("exit code=%d, want 125", code)
	}

	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected archive: %v", err)
	}
}

func TestCaptureV3PreflightRejectsExistingOutput(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	setupPortableCaptureV3Test(t)

	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	original := []byte("DO_NOT_OVERWRITE")
	if err := os.WriteFile(output, original, 0600); err != nil {
		t.Fatal(err)
	}

	code := captureCommandV3(
		[]string{"go", "build", "./..."},
		workspace,
		[]string{"missing-file.go"},
		output,
	)

	if code != 125 {
		t.Fatalf("exit code=%d, want 125", code)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(original) {
		t.Fatalf("existing archive changed: %q", got)
	}
}

func TestCaptureV3PreflightOutputCheck(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "failure.bfc")

	if err := checkCaptureOutputAvailableV3(output); err != nil {
		t.Fatalf("unexpected error for available output: %v", err)
	}

	original := []byte("KEEP_EXISTING_FILE")
	if err := os.WriteFile(output, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := checkCaptureOutputAvailableV3(output); err == nil {
		t.Fatal("existing output unexpectedly accepted")
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(original) {
		t.Fatalf("existing output changed: %q", got)
	}
}
