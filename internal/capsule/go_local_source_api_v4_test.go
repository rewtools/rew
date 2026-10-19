package capsule

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverLocalGoSourcesV4WithExecutable(t *testing.T) {
	goExecutable, err := ResolveGoExecutable()
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	if err := os.MkdirAll(app, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lib, "assets"), 0755); err != nil {
		t.Fatal(err)
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

import _ "embed"

//go:embed assets/message.txt
var message string

func Hello() { _ = message }
`)
	write(filepath.Join(lib, "assets", "message.txt"), "hello")

	cache, err := InspectGoModuleCacheWithExecutable(
		app, goExecutable,
	)
	if err != nil {
		t.Fatal(err)
	}

	mainFiles, localFiles, err := DiscoverLocalGoSourcesV4WithExecutable(
		app,
		"../lib",
		cache,
		goExecutable,
		GoBuildEnvironment{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	wantMain := []string{"go.mod", "main.go"}
	wantLocal := []string{
		"assets/message.txt",
		"go.mod",
		"lib.go",
	}

	if !reflect.DeepEqual(mainFiles, wantMain) {
		t.Fatalf("main: got %v, want %v", mainFiles, wantMain)
	}
	if !reflect.DeepEqual(localFiles, wantLocal) {
		t.Fatalf("local: got %v, want %v", localFiles, wantLocal)
	}

	files, err := ReadPinnedLocalModulesV4(
		app, "../lib", mainFiles, localFiles,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 5 {
		t.Fatalf("captured %d files, want 5", len(files))
	}
}
