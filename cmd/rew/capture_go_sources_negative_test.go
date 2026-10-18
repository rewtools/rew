package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV3GoSourcesRejectsDuplicateFlag(t *testing.T) {
	got := run([]string{
		"capture", "--v3",
		"--include-go-sources",
		"--include-go-sources",
		"--", "go", "build", "./...",
	})

	if got != 2 {
		t.Fatalf("duplicate flag exit = %d, want 2", got)
	}
}

func TestCaptureV3GoSourcesRejectsMissingSourcesBeforeExecution(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/empty-source\n\ngo 1.26.0\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(root, "command-executed")

	got := captureCommandV3WithDiscovery(
		[]string{"go", "build", "./..."},
		root,
		nil,
		filepath.Join(root, "failure.bfc"),
		false,
		true,
	)

	if got != 125 {
		t.Fatalf("missing sources exit = %d, want 125", got)
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unexpected marker: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "failure.bfc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected capsule: %v", err)
	}
}

func TestCaptureV3GoSourcesRejectsDuplicateExplicitInclude(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	files := map[string]string{
		"go.mod":  "module example.com/duplicate-source\n\ngo 1.26.0\n",
		"main.go": "package main\nfunc main() { undefinedFunction() }\n",
	}

	for name, content := range files {
		if err := os.WriteFile(
			filepath.Join(root, name),
			[]byte(content),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	marker := filepath.Join(root, "command-executed")

	got := captureCommandV3WithDiscovery(
		[]string{"go", "build", "./..."},
		root,
		[]string{"main.go", "main.go"},
		filepath.Join(root, "failure.bfc"),
		false,
		true,
	)

	if got != 125 {
		t.Fatalf("duplicate explicit include exit = %d, want 125", got)
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unexpected marker: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "failure.bfc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected capsule: %v", err)
	}
}

func TestCaptureV3GoSourcesPreflightDoesNotExecuteCommand(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/preflight-test\n\ngo 1.26.0\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(root, "command-executed")
	script := filepath.Join(root, "write-marker.sh")

	// This script is an execution sentinel. It must never run:
	// source discovery must reject this workspace first.
	content := "#!/bin/sh\n" +
		"printf executed > \"" + marker + "\"\n" +
		"exit 1\n"

	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "failure.bfc")

	got := captureCommandV3WithDiscovery(
		[]string{script},
		root,
		nil,
		output,
		false,
		true,
	)

	if got != 125 {
		t.Fatalf("preflight exit = %d, want 125", got)
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("preflight executed user command: marker err=%v", err)
	}

	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight created a capsule: err=%v", err)
	}
}
