package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureV3SensitiveGoEmbedPreflightIntegration(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)

	temp := t.TempDir()
	workspace := filepath.Join(temp, "workspace")

	if err := os.MkdirAll(
		filepath.Join(workspace, "config"), 0700,
	); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"go.mod":                 "module example.com/sensitive-embed-test\n\ngo 1.25\n",
		"main.go":                "package main\n\nimport (\n\t\"embed\"\n\t\"fmt\"\n)\n\n//go:embed config/.env.production\nvar assets embed.FS\n\nfunc main() {\n\tfmt.Println(assets)\n}\n",
		"config/.env.production": "SYNTHETIC_SECRET_NOT_FOR_OUTPUT=12345\n",
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

	binary := filepath.Join(temp, "rew")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."

	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build REW CLI: %v\n%s", err, output)
	}

	// Resolve the real Go executable before installing the test wrapper.
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	realGo, err = filepath.Abs(realGo)
	if err != nil {
		t.Fatal(err)
	}

	wrapperDir := filepath.Join(temp, "go-wrapper")
	if err := os.MkdirAll(wrapperDir, 0700); err != nil {
		t.Fatal(err)
	}

	buildMarker := filepath.Join(temp, "main-build-invoked")
	listMarker := filepath.Join(temp, "go-list-invoked")
	wrapper := filepath.Join(wrapperDir, "go")

	wrapperScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "list" ]; then
  : > %q
fi
if [ "$1" = "build" ]; then
  : > %q
fi
exec %q "$@"
`, listMarker, buildMarker, realGo)

	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		binary,
		"capture",
		"--v3",
		"--include-go-sources",
		"--include-go-embed",
		"--",
		"go",
		"build",
		"./...",
	)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"PATH="+wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CGO_ENABLED=0",
		"GOFLAGS=",
		"GOWORK=off",
		"GOTOOLCHAIN=local",
	)

	output, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v:\n%s", err, output)
	}

	if exitErr.ExitCode() != 125 {
		t.Fatalf("expected exit 125, got %d:\n%s",
			exitErr.ExitCode(), output)
	}

	diagnostic := string(output)

	if !strings.Contains(
		diagnostic, "sensitive workspace path",
	) {
		t.Fatalf("missing path preflight diagnostic:\n%s", diagnostic)
	}

	if !strings.Contains(
		diagnostic, "config/.env.production",
	) {
		t.Fatalf("missing blocked filename:\n%s", diagnostic)
	}

	if strings.Contains(
		diagnostic, "SYNTHETIC_SECRET_NOT_FOR_OUTPUT",
	) {
		t.Fatal("secret contents leaked into diagnostic")
	}

	if _, err := os.Stat(listMarker); err != nil {
		t.Fatalf("Go discovery wrapper was not invoked: %v", err)
	}

	if _, err := os.Stat(buildMarker); !os.IsNotExist(err) {
		t.Fatalf("main go build was invoked before path rejection: %v", err)
	}

	if _, err := os.Stat(
		filepath.Join(workspace, "failure.bfc"),
	); !os.IsNotExist(err) {
		t.Fatalf("capsule created despite rejection: %v", err)
	}
}
