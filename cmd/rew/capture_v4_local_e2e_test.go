package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV4LocalReplaceEndToEnd(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{app, lib} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"),
		"module example.com/app\n\n"+
			"go 1.25\n\n"+
			"require example.com/local-lib v0.0.0\n\n"+
			"replace example.com/local-lib => ../lib\n",
	)

	write(filepath.Join(app, "main.go"),
		"package main\n\n"+
			"import _ \"example.com/local-lib\"\n\n"+
			"func main() {}\n",
	)

	write(filepath.Join(lib, "go.mod"),
		"module example.com/local-lib\n\ngo 1.25\n",
	)

	write(filepath.Join(lib, "lib.go"),
		"package lib\n\nfunc Broken( {\n",
	)

	output := filepath.Join(parent, "failure.rew")

	code := captureCommandV4(
		[]string{"go", "build", "./..."},
		app,
		output,
	)
	if code != 1 {
		t.Fatalf("capture exit=%d, want 1", code)
	}

	verified, err := capsule.ReadVerifiedV4(output)
	if err != nil {
		t.Fatalf("verify captured V4 capsule: %v", err)
	}

	if verified.Manifest.SchemaVersion != capsule.SchemaVersionV4 {
		t.Fatal("unexpected capsule schema")
	}

	if verified.Manifest.Execution.ExitCode != 1 {
		t.Fatalf(
			"recorded exit=%d, want 1",
			verified.Manifest.Execution.ExitCode,
		)
	}

	if verified.Manifest.Execution.WorkingDir != "app" {
		t.Fatal("unexpected V4 execution working directory")
	}

	if !strings.Contains(
		verified.Manifest.Execution.Stderr,
		"lib.go",
	) {
		t.Fatalf(
			"recorded failure does not mention lib.go: %q",
			verified.Manifest.Execution.Stderr,
		)
	}

	found := make(map[string]bool)
	for _, file := range verified.Files {
		found[file.Path] = true
	}

	for _, required := range []string{
		"app/go.mod",
		"app/main.go",
		"lib/go.mod",
		"lib/lib.go",
	} {
		if !found[required] {
			t.Fatalf("missing captured file %q", required)
		}
	}

	if len(verified.Manifest.LocalModules) != 1 {
		t.Fatal("expected exactly one local module")
	}

	if verified.Manifest.LocalModules[0].ModulePath !=
		"example.com/local-lib" {
		t.Fatal("incorrect captured local module identity")
	}

	if len(verified.Manifest.GoSelectedModules) != 1 ||
		verified.Manifest.GoSelectedModules[0].Replace == nil ||
		verified.Manifest.GoSelectedModules[0].Replace.Path != "../lib" {
		t.Fatal("incorrect captured local replacement graph")
	}
}
