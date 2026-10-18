package capsule

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverLocalGoSourcesV4DetectsGoModMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell wrapper")
	}

	realGo, err := ResolveGoExecutable()
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{app, lib} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(name, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(name, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"), `module example.com/app

go 1.25

require example.com/lib v0.0.0

replace example.com/lib => ../lib
`, 0600)

	write(filepath.Join(app, "main.go"), `package main

import "example.com/lib"

func main() { lib.Hello() }
`, 0600)

	write(filepath.Join(lib, "go.mod"), `module example.com/lib

go 1.25
`, 0600)

	write(filepath.Join(lib, "lib.go"), `package lib

func Hello() {}
`, 0600)

	cache, err := InspectGoModuleCacheWithExecutable(
		app, realGo,
	)
	if err != nil {
		t.Fatal(err)
	}

	wrapper := filepath.Join(parent, "go-wrapper")
	localGoMod := filepath.Join(lib, "go.mod")

	// The first graph command runs normally. Immediately afterwards,
	// mutate the local go.mod in place while preserving its pathname.
	// All following discovery commands use the real Go executable.
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = list ] && [ \"$2\" = -m ]; then\n" +
		"  " + shellQuoteGoMutationV4(realGo) + " \"$@\"\n" +
		"  status=$?\n" +
		"  printf '\\n// changed during discovery\\n' >> " +
		shellQuoteGoMutationV4(localGoMod) + "\n" +
		"  exit \"$status\"\n" +
		"fi\n" +
		"exec " + shellQuoteGoMutationV4(realGo) + " \"$@\"\n"

	write(wrapper, script, 0700)

	_, _, err = DiscoverLocalGoSourcesV4WithExecutable(
		app,
		"../lib",
		cache,
		wrapper,
		GoBuildEnvironment{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	)

	if err == nil {
		t.Fatal("expected go.mod mutation rejection")
	}

	if !strings.Contains(err.Error(), "module manifests changed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func shellQuoteGoMutationV4(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
