package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureV3GoEmbedRejectsInvalidResources(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	for _, tc := range []struct {
		name    string
		symlink bool
	}{
		{name: "missing embedded resource"},
		{name: "symlink embedded resource", symlink: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()

			if err := os.MkdirAll(
				filepath.Join(root, "assets"), 0700,
			); err != nil {
				t.Fatal(err)
			}

			source := `package main

import _ "embed"

//go:embed assets/message.txt
var message string

func main() {
    _ = message
    rewIntentionallyUndefined()
}
`
			files := map[string]string{
				"go.mod":  "module example.com/embed-negative\n\ngo 1.26\n",
				"main.go": source,
			}

			for name, contents := range files {
				if err := os.WriteFile(
					filepath.Join(root, name),
					[]byte(contents), 0600,
				); err != nil {
					t.Fatal(err)
				}
			}

			if tc.symlink {
				outside := filepath.Join(t.TempDir(), "secret.txt")
				if err := os.WriteFile(
					outside, []byte("outside workspace\n"), 0600,
				); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(
					outside, filepath.Join(root, "assets", "message.txt"),
				); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}

			marker := filepath.Join(root, "command-executed")
			output := filepath.Join(root, "failure.bfc")

			realGo, err := exec.LookPath("go")
			if err != nil {
				t.Fatal(err)
			}
			realGo, err = filepath.Abs(realGo)
			if err != nil {
				t.Fatal(err)
			}

			binDir := t.TempDir()
			wrapper := filepath.Join(binDir, "go")

			scriptBody := `#!/bin/sh
if [ "$1" = "build" ]; then
    printf executed > "$REW_TEST_MARKER"
fi
exec "$REW_TEST_REAL_GO" "$@"
`
			if err := os.WriteFile(
				wrapper, []byte(scriptBody), 0700,
			); err != nil {
				t.Fatal(err)
			}

			t.Setenv("REW_TEST_REAL_GO", realGo)
			t.Setenv("REW_TEST_MARKER", marker)
			t.Setenv(
				"PATH",
				binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
			)

			stderrRead, stderrWrite, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}

			originalStderr := os.Stderr
			os.Stderr = stderrWrite

			code := captureCommandV3WithEmbed(
				[]string{"go", "build", "./..."},
				root,
				[]string{"go.mod", "main.go"},
				output,
				true,
			)

			os.Stderr = originalStderr
			if err := stderrWrite.Close(); err != nil {
				t.Fatal(err)
			}

			diagnostics, err := io.ReadAll(stderrRead)
			if err != nil {
				t.Fatal(err)
			}
			if err := stderrRead.Close(); err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(
				string(diagnostics),
				"incomplete Go embed package metadata",
			) {
				t.Fatalf("unexpected Capture rejection: %q", diagnostics)
			}

			if code != 125 {
				t.Fatalf("capture exit = %d, want 125", code)
			}

			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("capsule unexpectedly exists or stat failed: %v", err)
			}

			if _, err := os.Lstat(marker); !os.IsNotExist(err) {
				t.Fatalf("unexpected command marker: %v", err)
			}
		})
	}
}
