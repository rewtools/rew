package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV4DoesNotOverwriteExistingCapsule(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "failure.rew")

	original := []byte("existing capsule must remain unchanged\n")

	if err := os.WriteFile(output, original, 0600); err != nil {
		t.Fatal(err)
	}

	// The workspace is intentionally not a Go module.
	// Output protection must happen before Go inspection.
	code := captureCommandV4(
		[]string{"go", "build", "./..."},
		dir,
		output,
	)

	if code != 125 {
		t.Fatalf("capture exit=%d, want 125", code)
	}

	actual, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(actual, original) {
		t.Fatalf(
			"existing capsule changed: got %q, want %q",
			actual, original,
		)
	}
}

func TestCaptureV4DoesNotFollowOutputSymlink(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, "existing.txt")
	output := filepath.Join(dir, "failure.rew")

	original := []byte("protected target\n")

	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(target, output); err != nil {
		t.Fatal(err)
	}

	code := captureCommandV4(
		[]string{"go", "build", "./..."},
		dir,
		output,
	)

	if code != 125 {
		t.Fatalf("capture exit=%d, want 125", code)
	}

	actual, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(actual, original) {
		t.Fatalf(
			"symlink target changed: got %q, want %q",
			actual, original,
		)
	}

	info, err := os.Lstat(output)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("output symlink unexpectedly replaced")
	}
}
