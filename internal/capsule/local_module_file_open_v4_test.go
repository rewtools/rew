package capsule

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenPinnedRegularFileV4(t *testing.T) {
	setup := func(t *testing.T) *os.Root {
		t.Helper()

		base := t.TempDir()

		for _, dir := range []string{
			"pkg",
			"pkg/sub",
			"other",
		} {
			if err := os.Mkdir(
				filepath.Join(base, dir), 0700,
			); err != nil {
				t.Fatal(err)
			}
		}

		for name, data := range map[string]string{
			"pkg/sub/source.go": "original",
			"other/source.go":   "other",
		} {
			if err := os.WriteFile(
				filepath.Join(base, name),
				[]byte(data),
				0600,
			); err != nil {
				t.Fatal(err)
			}
		}

		root, err := os.OpenRoot(base)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := root.Close(); err != nil {
				t.Errorf("close root: %v", err)
			}
		})

		return root
	}

	t.Run("opens nested regular file", func(t *testing.T) {
		root := setup(t)

		file, err := openPinnedRegularFileV4(
			root, "pkg/sub/source.go",
		)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "original" {
			t.Fatalf("read %q, want original", data)
		}
	})

	t.Run("rejects symlink in intermediate path", func(t *testing.T) {
		root := setup(t)

		if err := root.Symlink(
			"../other", "pkg/linked",
		); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}

		file, err := openPinnedRegularFileV4(
			root, "pkg/linked/source.go",
		)
		if file != nil {
			file.Close()
		}
		if err == nil {
			t.Fatal("expected intermediate symlink rejection")
		}
	})

	t.Run("rejects final symlink", func(t *testing.T) {
		root := setup(t)

		if err := root.Symlink(
			"source.go", "pkg/sub/linked.go",
		); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}

		file, err := openPinnedRegularFileV4(
			root, "pkg/sub/linked.go",
		)
		if file != nil {
			file.Close()
		}
		if err == nil {
			t.Fatal("expected final symlink rejection")
		}
	})

	t.Run("rejects traversal", func(t *testing.T) {
		root := setup(t)

		file, err := openPinnedRegularFileV4(
			root, "pkg/../other/source.go",
		)
		if file != nil {
			file.Close()
		}
		if err == nil {
			t.Fatal("expected noncanonical path rejection")
		}
	})

	t.Run("rejects directories as files", func(t *testing.T) {
		root := setup(t)

		file, err := openPinnedRegularFileV4(root, "pkg/sub")
		if file != nil {
			file.Close()
		}
		if err == nil {
			t.Fatal("expected directory rejection")
		}
	})
}
