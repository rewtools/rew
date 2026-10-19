package capsule

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDualModuleGoSourceDiscoveryV4Integration(t *testing.T) {
	goExecutable, err := ResolveGoExecutable()
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{
		app,
		lib,
		filepath.Join(app, "cmd", "worker"),
		filepath.Join(lib, "internal", "helper"),
		filepath.Join(lib, "assets"),
	} {
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

import "example.com/lib"

func main() { lib.Hello() }
`)
	write(filepath.Join(app, "cmd", "worker", "main.go"), `package main

func main() {}
`)
	write(filepath.Join(lib, "go.mod"), `module example.com/lib

go 1.25
`)
	write(filepath.Join(lib, "lib.go"), `package lib

import (
	_ "embed"
	"example.com/lib/internal/helper"
)

//go:embed assets/message.txt
var message string

func Hello() {
	helper.Use()
	_ = message
}
`)
	write(filepath.Join(lib, "internal", "helper", "helper.go"), `package helper

func Use() {}
`)
	write(filepath.Join(lib, "assets", "message.txt"), "embedded content")

	cache, err := InspectGoModuleCacheWithExecutable(
		app, goExecutable,
	)
	if err != nil {
		t.Fatal(err)
	}

	target := GoBuildEnvironment{
		GOOS:   "linux",
		GOARCH: "amd64",
	}

	mainSources, err := DiscoverGoSourceFilesV3WithExecutable(
		app, cache, goExecutable, target,
	)
	if err != nil {
		t.Fatal(err)
	}

	localSources, err := DiscoverGoSourceFilesV3WithExecutable(
		lib, cache, goExecutable, target,
	)
	if err != nil {
		t.Fatal(err)
	}

	mainEmbeds, err := DiscoverGoEmbedFilesV3WithExecutable(
		app, cache, goExecutable, target,
	)
	if err != nil {
		t.Fatal(err)
	}

	localEmbeds, err := DiscoverGoEmbedFilesV3WithExecutable(
		lib, cache, goExecutable, target,
	)
	if err != nil {
		t.Fatal(err)
	}

	mainFiles, localFiles, err := MergeLocalGoSourcesV4(
		append(mainSources, mainEmbeds...),
		append(localSources, localEmbeds...),
	)
	if err != nil {
		t.Fatal(err)
	}

	wantMain := []string{
		"cmd/worker/main.go",
		"go.mod",
		"main.go",
	}
	wantLocal := []string{
		"assets/message.txt",
		"go.mod",
		"internal/helper/helper.go",
		"lib.go",
	}

	if !reflect.DeepEqual(mainFiles, wantMain) {
		t.Fatalf("main files: got %v, want %v", mainFiles, wantMain)
	}
	if !reflect.DeepEqual(localFiles, wantLocal) {
		t.Fatalf("local files: got %v, want %v", localFiles, wantLocal)
	}

	files, err := ReadPinnedLocalModulesV4(
		app, "../lib", mainFiles, localFiles,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != len(mainFiles)+len(localFiles) {
		t.Fatalf("unexpected captured file count: %d", len(files))
	}

	t.Logf("main files: %v", mainFiles)
	t.Logf("local files: %v", localFiles)
	t.Logf("pinned workspace files: %d", len(files))
}
