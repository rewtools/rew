package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/execution"
)

func TestCaptureV4RejectsMutationDuringExecution(t *testing.T) {
	tests := []struct {
		name   string
		module string
		file   string
	}{
		{
			name:   "main module source modified",
			module: "app",
			file:   "main.go",
		},
		{
			name:   "local module source modified",
			module: "lib",
			file:   "lib.go",
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

			write := func(name, data string) {
				t.Helper()
				if err := os.WriteFile(
					name, []byte(data), 0600,
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

			mutatedPath := filepath.Join(
				parent, tc.module, tc.file,
			)

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
					t.Fatal("executor invoked more than once")
				}

				if len(argv) != 3 ||
					argv[0] != "go" ||
					argv[1] != "build" ||
					argv[2] != "./..." {
					t.Fatalf("unexpected argv: %v", argv)
				}

				if dir != app {
					t.Fatalf("unexpected execution dir: %s", dir)
				}

				if !filepath.IsAbs(executable) {
					t.Fatal("Go executable is not absolute")
				}

				// This callback runs after Capture V4 has read
				// both pinned modules into its initial snapshot.
				write(
					mutatedPath,
					"package changed\n\nfunc Changed() {}\n",
				)

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
					"capsule published despite workspace mutation: %v",
					err,
				)
			}

			content, err := os.ReadFile(mutatedPath)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(
				string(content), "func Changed()",
			) {
				t.Fatal("executor did not mutate target source")
			}
		})
	}
}
