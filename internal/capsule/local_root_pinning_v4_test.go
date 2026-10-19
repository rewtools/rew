package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalRootPinningV4(t *testing.T) {
	parent := t.TempDir()
	lib := filepath.Join(parent, "lib")

	if err := os.Mkdir(lib, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(lib, "identity.txt"),
		[]byte("original"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	parentRoot, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer parentRoot.Close()

	libRoot, err := parentRoot.OpenRoot("lib")
	if err != nil {
		t.Fatal(err)
	}
	defer libRoot.Close()

	if err := os.Rename(lib, filepath.Join(parent, "old-lib")); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(lib, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(lib, "identity.txt"),
		[]byte("replacement"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	data, err := libRoot.ReadFile("identity.txt")
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "original" {
		t.Fatalf("pinned root read %q, want original", data)
	}

	t.Run("rejects escape", func(t *testing.T) {
		if _, err := libRoot.ReadFile("../lib/identity.txt"); err == nil {
			t.Fatal("expected traversal outside pinned root to fail")
		}
	})

	t.Run("fresh root sees replacement", func(t *testing.T) {
		fresh, err := parentRoot.OpenRoot("lib")
		if err != nil {
			t.Fatal(err)
		}
		defer fresh.Close()

		current, err := fresh.ReadFile("identity.txt")
		if err != nil {
			t.Fatal(err)
		}
		if string(current) != "replacement" {
			t.Fatalf("fresh root read %q, want replacement", current)
		}
	})
}
