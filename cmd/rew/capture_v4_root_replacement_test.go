package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/execution"
)

func TestCaptureV4RejectsLocalRootReplacementDuringExecution(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")
	oldLib := filepath.Join(parent, "lib-old")

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

	localGoMod := "module example.com/local-lib\n\ngo 1.25\n"
	localSource := "package lib\n\nfunc Broken( {\n"

	write(filepath.Join(lib, "go.mod"), localGoMod)
	write(filepath.Join(lib, "lib.go"), localSource)

	output := filepath.Join(parent, "failure.rew")
	executorCalls := 0

	executor := func(
		argv []string,
		executable string,
		dir string,
		env []string,
		stdout, stderr io.Writer,
	) (execution.Result, error) {
		executorCalls++

		if executorCalls != 1 {
			t.Fatal("executor called more than once")
		}

		if dir != app {
			t.Fatalf("unexpected execution directory: %s", dir)
		}

		// Move the original pinned directory away.
		if err := os.Rename(lib, oldLib); err != nil {
			t.Fatal(err)
		}

		// Create a different physical directory at the same path,
		// containing identical Go module files and bytes.
		if err := os.Mkdir(lib, 0700); err != nil {
			t.Fatal(err)
		}

		write(filepath.Join(lib, "go.mod"), localGoMod)
		write(filepath.Join(lib, "lib.go"), localSource)

		originalInfo, err := os.Stat(oldLib)
		if err != nil {
			t.Fatal(err)
		}

		currentInfo, err := os.Stat(lib)
		if err != nil {
			t.Fatal(err)
		}

		if os.SameFile(originalInfo, currentInfo) {
			t.Fatal("replacement directory has same identity")
		}

		for _, name := range []string{"go.mod", "lib.go"} {
			original, err := os.ReadFile(filepath.Join(oldLib, name))
			if err != nil {
				t.Fatal(err)
			}

			replacement, err := os.ReadFile(filepath.Join(lib, name))
			if err != nil {
				t.Fatal(err)
			}

			if string(original) != string(replacement) {
				t.Fatalf("replacement content differs: %s", name)
			}
		}

		return execution.Result{
			Argv:     append([]string(nil), argv...),
			ExitCode: 1,
			Stderr:   "simulated build failure\n",
		}, nil
	}

	code := captureCommandV4WithExecutor(
		[]string{"go", "build", "./..."},
		app,
		output,
		executor,
	)

	if executorCalls != 1 {
		t.Fatalf("executor calls=%d, want 1", executorCalls)
	}

	if code != 125 {
		t.Fatalf("capture exit=%d, want 125", code)
	}

	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("capsule published after root replacement: %v", err)
	}

	if _, err := os.Stat(oldLib); err != nil {
		t.Fatalf("original directory missing: %v", err)
	}

	if _, err := os.Stat(lib); err != nil {
		t.Fatalf("replacement directory missing: %v", err)
	}

	// Verify the replacement retained identical source bytes.
	content, err := os.ReadFile(filepath.Join(lib, "lib.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "func Broken(") {
		t.Fatal("replacement source does not match original fixture")
	}
}
