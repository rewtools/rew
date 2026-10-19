package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV4RejectsLocalModuleMutation(t *testing.T) {
	// This test invokes the same pinned-session verification used
	// by Capture V4. It does not rely on timing or background sleeps.
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
		"module example.com/app\n\ngo 1.25\n\n"+
			"require example.com/local-lib v0.0.0\n\n"+
			"replace example.com/local-lib => ../lib\n",
	)

	write(filepath.Join(app, "main.go"),
		"package main\n\nfunc main() {}\n",
	)

	write(filepath.Join(lib, "go.mod"),
		"module example.com/local-lib\n\ngo 1.25\n",
	)

	localSource := filepath.Join(lib, "lib.go")

	write(localSource,
		"package lib\n\nfunc Original() {}\n",
	)

	session, err := capsule.OpenPinnedLocalModuleSessionV4(
		app, "../lib",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	mainPaths := []string{"go.mod", "main.go"}
	localPaths := []string{"go.mod", "lib.go"}

	before, err := session.Read(mainPaths, localPaths)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a source mutation occurring during execution.
	write(localSource,
		"package lib\n\nfunc Changed() {}\n",
	)

	if err := session.Verify(
		mainPaths, localPaths, before,
	); err == nil {
		t.Fatal("mutated local module accepted by pinned verification")
	}

	// An unsuccessful capture must not publish an archive.
	output := filepath.Join(parent, "failure.rew")
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected capsule: %v", err)
	}
}
