package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLocalModuleRootsV4(t *testing.T) {
	makeFixture := func(t *testing.T) (string, string) {
		t.Helper()

		parent := t.TempDir()
		app := filepath.Join(parent, "app")
		lib := filepath.Join(parent, "lib")

		for _, dir := range []string{app, lib} {
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
		}

		return app, lib
	}

	t.Run("accepts sibling modules", func(t *testing.T) {
		app, lib := makeFixture(t)

		got, err := ResolveLocalModuleRootV4(app, "../lib")
		if err != nil {
			t.Fatal(err)
		}
		if got != lib {
			t.Fatalf("root=%q, want=%q", got, lib)
		}
	})

	t.Run("rejects missing local module", func(t *testing.T) {
		app, _ := makeFixture(t)

		if _, err := ResolveLocalModuleRootV4(
			app, "../missing",
		); err == nil {
			t.Fatal("expected missing module rejection")
		}
	})

	t.Run("rejects symlink local root", func(t *testing.T) {
		app, lib := makeFixture(t)
		link := filepath.Join(filepath.Dir(app), "linked")
		if err := os.Symlink(lib, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}

		if _, err := ResolveLocalModuleRootV4(
			app, "../linked",
		); err == nil {
			t.Fatal("expected symlink rejection")
		}
	})

	t.Run("rejects parent escape", func(t *testing.T) {
		app, _ := makeFixture(t)

		if _, err := ResolveLocalModuleRootV4(
			app, "../../outside",
		); err == nil {
			t.Fatal("expected parent escape rejection")
		}
	})

	t.Run("rejects noncanonical path", func(t *testing.T) {
		app, _ := makeFixture(t)

		if _, err := ResolveLocalModuleRootV4(
			app, ".././lib",
		); err == nil {
			t.Fatal("expected noncanonical path rejection")
		}
	})

	t.Run("rejects file instead of directory", func(t *testing.T) {
		app, _ := makeFixture(t)
		file := filepath.Join(filepath.Dir(app), "not-directory")

		if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}

		if _, err := ResolveLocalModuleRootV4(
			app, "../not-directory",
		); err == nil {
			t.Fatal("expected non-directory rejection")
		}
	})
}
