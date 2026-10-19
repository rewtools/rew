package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV4CLIRejectsUnsupportedArgumentShapes(t *testing.T) {
	cases := [][]string{
		{"capture", "--v4"},
		{"capture", "--v4", "--", "go", "build", "./..."},
		{"capture", "--v4", "--include-go-sources", "--"},
		{"capture", "--v4", "--include-go-sources", "--", "go", "test", "./..."},
		{"capture", "--v4", "--include-go-sources", "--", "go", "build", "."},
		{"capture", "--v4", "--include-go-sources", "--", "go", "build", "./...", "-overlay=x"},
	}

	for _, args := range cases {
		if code := run(args); code != 2 {
			t.Fatalf("args=%v: exit=%d, want 2", args, code)
		}
	}
}

func TestCaptureV4CLIRoutesValidArgumentsToCapture(t *testing.T) {
	// A valid invocation must reach the capture preflight, rather
	// than being rejected by CLI argument parsing.
	// Existing output makes that preflight return 125 safely.
	dir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	output := filepath.Join(dir, "failure.rew")
	if err := os.WriteFile(output, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}

	code := run([]string{
		"capture",
		"--v4",
		"--include-go-sources",
		"--",
		"go", "build", "./...",
	})
	if code != 125 {
		t.Fatalf("valid V4 route: exit=%d, want 125", code)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "preserve" {
		t.Fatal("V4 routing modified existing capsule")
	}
}
