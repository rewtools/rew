package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV4RejectsSymlinkedLocalSource(t *testing.T) {
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
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
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

	external := filepath.Join(parent, "secret.go")
	write(external, "package lib\n")

	if err := os.Symlink(
		external, filepath.Join(lib, "lib.go"),
	); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(parent, "failure.rew")

	code := captureCommandV4(
		[]string{"go", "build", "./..."},
		app,
		output,
	)

	if code != 125 {
		t.Fatalf("capture exit=%d, want 125", code)
	}

	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected output capsule: %v", err)
	}
}
