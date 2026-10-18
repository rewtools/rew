package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureV3SensitiveEnvironmentPreflightIntegration(t *testing.T) {
	temp := t.TempDir()
	marker := filepath.Join(temp, "go-was-executed")
	fakeGo := filepath.Join(temp, "go")
	capsulePath := filepath.Join(temp, "failure.bfc")

	script := "#!/bin/sh\n" +
		"touch " + marker + "\n" +
		"exit 99\n"

	if err := os.WriteFile(fakeGo, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	cliBinary := filepath.Join(temp, "rew")

	build := exec.Command(
		"go", "build", "-o", cliBinary, ".",
	)
	build.Env = os.Environ()

	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build REW: %v\n%s", err, output)
	}

	command := exec.Command(
		cliBinary,
		"capture",
		"--v3",
		"--include-go-sources",
		"--",
		"go",
		"build",
		"./...",
	)

	command.Dir = temp
	command.Env = isolatedCaptureEnvironmentV3(
		temp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"integration-secret-must-not-leak",
	)

	output, err := command.CombinedOutput()

	var exitErr *exec.ExitError
	if !isExpectedExitCode(err, &exitErr, 125) {
		t.Fatalf("expected exit 125, got %v; output: %s", err, output)
	}

	if !strings.Contains(string(output), "GITHUB_TOKEN") {
		t.Fatalf("missing credential name: %s", output)
	}

	if strings.Contains(string(output), "integration-secret-must-not-leak") {
		t.Fatal("credential value leaked into diagnostic output")
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Go executable was invoked: %v", err)
	}

	if _, err := os.Stat(capsulePath); !os.IsNotExist(err) {
		t.Fatalf("capsule unexpectedly created: %v", err)
	}
}

func isExpectedExitCode(
	err error,
	exitErr **exec.ExitError,
	expected int,
) bool {
	return errors.As(err, exitErr) &&
		(*exitErr).ExitCode() == expected
}

func isolatedCaptureEnvironmentV3(
	path string,
	tokenValue string,
) []string {
	return []string{
		"PATH=" + path,
		"HOME=" + path,
		"TMPDIR=" + path,
		"GITHUB_TOKEN=" + tokenValue,
	}
}

func TestCaptureV3EmptySensitiveEnvironmentContinues(t *testing.T) {
	temp := t.TempDir()
	marker := filepath.Join(temp, "go-was-executed")
	fakeGo := filepath.Join(temp, "go")
	cliBinary := filepath.Join(temp, "rew")

	script := "#!/bin/sh\n" +
		"printf invoked > " + marker + "\n" +
		"exit 99\n"

	if err := os.WriteFile(fakeGo, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	build := exec.Command("go", "build", "-o", cliBinary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build REW: %v\n%s", err, output)
	}

	command := exec.Command(
		cliBinary,
		"capture",
		"--v3",
		"--include-go-sources",
		"--",
		"go", "build", "./...",
	)
	command.Dir = temp

	command.Env = isolatedCaptureEnvironmentV3(
		temp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"",
	)

	output, err := command.CombinedOutput()

	var exitErr *exec.ExitError
	if !isExpectedExitCode(err, &exitErr, 125) {
		t.Fatalf("expected Go resolution failure (125), got %v; output: %s", err, output)
	}

	if strings.Contains(string(output), "sensitive environment variable") {
		t.Fatalf("empty token should not be rejected: %s", output)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf(
			"expected Go runtime resolution to execute fake Go; command error: %v; output: %s",
			err, output,
		)
	}
}
