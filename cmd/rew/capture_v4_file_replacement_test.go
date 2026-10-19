package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/execution"
)

func TestCaptureV4RejectsFileReplacementDuringExecution(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{
			name: "main module",
			file: "app/main.go",
		},
		{
			name: "local module",
			file: "lib/lib.go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
				if err := os.WriteFile(
					name, []byte(content), 0600,
				); err != nil {
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
			target := filepath.Join(parent, filepath.FromSlash(tc.file))

			executorCalls := 0

			executor := func(
				argv []string,
				executable string,
				dir string,
				env []string,
				stdout, stderr io.Writer,
			) (execution.Result, error) {
				executorCalls++

				// Create a distinct regular file, then atomically
				// replace the captured file during execution.
				replacement := target + ".replacement"

				write(replacement,
					"package changed\n\nfunc Changed() {}\n",
				)

				if err := os.Rename(replacement, target); err != nil {
					t.Fatal(err)
				}

				return execution.Result{
					Argv:     append([]string(nil), argv...),
					ExitCode: 1,
					Stderr:   "simulated Go build failure\n",
				}, nil
			}

			code := captureCommandV4WithExecutor(
				[]string{"go", "build", "./..."},
				app,
				output,
				executor,
			)

			if executorCalls != 1 {
				t.Fatalf(
					"executor calls=%d, want 1",
					executorCalls,
				)
			}

			if code != 125 {
				t.Fatalf(
					"capture exit=%d, want 125",
					code,
				)
			}

			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf(
					"capsule published after replacement: %v",
					err,
				)
			}
		})
	}
}
