package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureV3SensitiveExplicitPathPreflightIntegration(t *testing.T) {
	temp := t.TempDir()
	workspace := filepath.Join(temp, "workspace")

	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}

	secret := "synthetic-sensitive-file-value-do-not-print"

	if err := os.WriteFile(
		filepath.Join(workspace, ".env"),
		[]byte(secret+"\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(temp, "rew")

	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build REW CLI: %v\n%s", err, output)
	}

	fakeBin := filepath.Join(temp, "bin")
	if err := os.MkdirAll(fakeBin, 0700); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(temp, "go-was-executed")

	fakeGo := filepath.Join(fakeBin, "go")
	script := "#!/bin/sh\n" +
		"touch " + marker + "\n" +
		"exit 99\n"

	if err := os.WriteFile(fakeGo, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		binary,
		"capture",
		"--v3",
		"--include",
		".env",
		"--",
		"go",
		"build",
		"./...",
	)

	cmd.Dir = workspace

	// Run in a controlled environment so unrelated credentials
	// cannot trigger the earlier environment preflight.
	cmd.Env = []string{
		"PATH=" + fakeBin,
		"HOME=" + temp,
		"TMPDIR=" + temp,
	}

	output, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v: %s", err, output)
	}

	if exitErr.ExitCode() != 125 {
		t.Fatalf("expected exit 125, got %d: %s",
			exitErr.ExitCode(), output)
	}

	diagnostic := string(output)

	if !strings.Contains(diagnostic, "sensitive workspace path") {
		t.Fatalf("missing sensitive path diagnostic: %s", diagnostic)
	}

	if strings.Contains(diagnostic, secret) {
		t.Fatal("secret file contents leaked into diagnostic")
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Go executable was invoked before preflight: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "failure.bfc")); !os.IsNotExist(err) {
		t.Fatalf("capsule was created despite rejection: %v", err)
	}
}
