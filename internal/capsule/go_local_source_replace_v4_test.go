package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverLocalGoSourcesV4RejectsMismatchedReplace(t *testing.T) {
	goExecutable, err := ResolveGoExecutable()
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")
	other := filepath.Join(parent, "other")

	for _, dir := range []string{app, lib, other} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"), `module example.com/app

go 1.25

require example.com/lib v0.0.0

replace example.com/lib => ../lib
`)

	write(filepath.Join(app, "main.go"), `package main

import "example.com/lib"

func main() { lib.Hello() }
`)

	write(filepath.Join(lib, "go.mod"), `module example.com/lib

go 1.25
`)

	write(filepath.Join(lib, "lib.go"), `package lib

func Hello() {}
`)

	write(filepath.Join(other, "go.mod"), `module example.com/other

go 1.25
`)

	write(filepath.Join(other, "other.go"), `package other

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
		"../other",
		cache,
		goExecutable,
		GoBuildEnvironment{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	)

	if err == nil {
		t.Fatal("expected mismatched replace rejection")
	}
}
