package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListGoModuleGraphV4WithExecutable(t *testing.T) {
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

	mainGoMod := `module example.com/app

go 1.25

require example.com/lib v0.0.0

replace example.com/lib => ../lib
`

	write(filepath.Join(app, "go.mod"), mainGoMod)
	write(filepath.Join(lib, "go.mod"), `module example.com/lib

go 1.25
`)

	cache, err := InspectGoModuleCacheWithExecutable(
		app, goExecutable,
	)
	if err != nil {
		t.Fatal(err)
	}

	modules, err := ListGoModuleGraphV4WithExecutable(
		app, cache, goExecutable,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(modules) != 1 {
		t.Fatalf("got %d modules, want 1", len(modules))
	}

	local := modules[0]

	if local.Path != "example.com/lib" ||
		local.Version != "v0.0.0" ||
		local.Replace == nil ||
		local.Replace.Path != "../lib" {
		t.Fatalf("unexpected selected graph: %+v", modules)
	}

	if err := ValidateGoModuleConsistencyV4(
		[]byte(mainGoMod), modules,
	); err != nil {
		t.Fatalf("graph consistency: %v", err)
	}
}
