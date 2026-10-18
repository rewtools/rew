package capsule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPinnedLocalModulesV4(t *testing.T) {
	setup := func(t *testing.T) (string, string, string) {
		t.Helper()

		parent := t.TempDir()
		app := filepath.Join(parent, "app")
		lib := filepath.Join(parent, "lib")

		for _, dir := range []string{app, lib} {
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
		}

		for name, data := range map[string]string{
			"app/go.mod":  "module example.com/app\n\ngo 1.25\n",
			"app/main.go": "package main\n",
			"lib/go.mod":  "module example.com/lib\n\ngo 1.25\n",
			"lib/lib.go":  "package lib\n",
		} {
			if err := os.WriteFile(
				filepath.Join(parent, name),
				[]byte(data),
				0600,
			); err != nil {
				t.Fatal(err)
			}
		}

		return parent, app, lib
	}

	t.Run("reads both modules", func(t *testing.T) {
		_, app, _ := setup(t)

		files, err := ReadPinnedLocalModulesV4(
			app,
			"../lib",
			[]string{"go.mod", "main.go"},
			[]string{"go.mod", "lib.go"},
		)
		if err != nil {
			t.Fatal(err)
		}

		if len(files) != 4 {
			t.Fatalf("files=%d, want 4", len(files))
		}

		want := []string{
			"app/go.mod",
			"app/main.go",
			"lib/go.mod",
			"lib/lib.go",
		}

		for i, path := range want {
			if files[i].Path != path {
				t.Fatalf("files[%d]=%q, want %q", i, files[i].Path, path)
			}
		}
	})

	t.Run("rejects symlink local root", func(t *testing.T) {
		parent, app, lib := setup(t)
		if err := os.Symlink(lib, filepath.Join(parent, "linked")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		if _, err := ReadPinnedLocalModulesV4(
			app, "../linked",
			[]string{"go.mod"},
			[]string{"go.mod"},
		); err == nil {
			t.Fatal("expected symlink root rejection")
		}
	})

	t.Run("rejects symlink file", func(t *testing.T) {
		_, app, lib := setup(t)
		if err := os.Symlink(
			"lib.go",
			filepath.Join(lib, "linked.go"),
		); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		if _, err := ReadPinnedLocalModulesV4(
			app, "../lib",
			[]string{"go.mod"},
			[]string{"go.mod", "linked.go"},
		); err == nil {
			t.Fatal("expected symlink file rejection")
		}
	})

	t.Run("rejects traversal", func(t *testing.T) {
		_, app, _ := setup(t)

		if _, err := ReadPinnedLocalModulesV4(
			app, "../lib",
			[]string{"go.mod"},
			[]string{"../lib/go.mod"},
		); err == nil {
			t.Fatal("expected traversal rejection")
		}
	})

	t.Run("rejects missing source", func(t *testing.T) {
		_, app, _ := setup(t)

		if _, err := ReadPinnedLocalModulesV4(
			app, "../lib",
			[]string{"go.mod"},
			[]string{"go.mod", "missing.go"},
		); err == nil {
			t.Fatal("expected missing source rejection")
		}
	})
}

func TestReadPinnedLocalModulesV4NestedSymlink(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{app, lib} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	for name, contents := range map[string]string{
		"app/go.mod": "module example.com/app\n",
		"lib/go.mod": "module example.com/lib\n",
	} {
		if err := os.WriteFile(
			filepath.Join(parent, name),
			[]byte(contents),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Mkdir(filepath.Join(lib, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(lib, "real", "source.go"),
		[]byte("package lib\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		"real",
		filepath.Join(lib, "linked"),
	); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := ReadPinnedLocalModulesV4(
		app,
		"../lib",
		[]string{"go.mod"},
		[]string{"go.mod", "linked/source.go"},
	); err == nil {
		t.Fatal("expected nested symlink rejection")
	}
}

func TestReadPinnedLocalModulesV4RejectsFileCountOverflow(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{app, lib} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	mainPaths := make([]string, MaxWorkspaceV3Files)
	for i := range mainPaths {
		mainPaths[i] = "go.mod"
	}

	_, err := ReadPinnedLocalModulesV4(
		app,
		"../lib",
		mainPaths,
		[]string{"go.mod"},
	)
	if err == nil {
		t.Fatal("expected combined file-count limit rejection")
	}
	if !strings.Contains(err.Error(), "too many v4 module files") {
		t.Fatalf("wrong rejection reason: %v", err)
	}
}
