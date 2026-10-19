package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverLocalGoSourcesV4RejectsWrongModuleIdentity(t *testing.T) {
	goExecutable, err := ResolveGoExecutable()
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

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"), `module example.com/app

go 1.25

require example.com/lib v0.0.0

replace example.com/lib => ../lib
`)

	write(filepath.Join(app, "main.go"), `package main

func main() {}
`)

	write(filepath.Join(lib, "go.mod"), `module example.com/other

go 1.25
`)

	write(filepath.Join(lib, "other.go"), `package other

func Hello() {}
`)

	cache, err := InspectGoModuleCacheWithExecutable(
		app, goExecutable,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = DiscoverLocalGoSourcesV4WithExecutable(
		app,
		"../lib",
		cache,
		goExecutable,
		GoBuildEnvironment{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	)

	if err == nil {
		t.Fatal("expected local module identity rejection")
	}
}
