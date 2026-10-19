package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedLocalModuleSessionReadVerifyV4(t *testing.T) {
	setup := func(t *testing.T) (string, string) {
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
			"app/go.mod":  "module example.com/app\n",
			"app/main.go": "package main\n",
			"lib/go.mod":  "module example.com/lib\n",
			"lib/lib.go":  "package lib\n",
		} {
			if err := os.WriteFile(filepath.Join(parent, name), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}

		return app, lib
	}

	mainPaths := []string{"go.mod", "main.go"}
	localPaths := []string{"go.mod", "lib.go"}

	t.Run("reads and verifies unchanged files", func(t *testing.T) {
		app, _ := setup(t)

		session, err := OpenPinnedLocalModuleSessionV4(app, "../lib")
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		before, err := session.Read(mainPaths, localPaths)
		if err != nil {
			t.Fatal(err)
		}
		if len(before) != 4 {
			t.Fatalf("got %d files, want 4", len(before))
		}

		if err := session.Verify(mainPaths, localPaths, before); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("detects local file mutation", func(t *testing.T) {
		app, lib := setup(t)

		session, err := OpenPinnedLocalModuleSessionV4(app, "../lib")
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		before, err := session.Read(mainPaths, localPaths)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(lib, "lib.go"),
			[]byte("package lib\n// changed\n"),
			0600,
		); err != nil {
			t.Fatal(err)
		}

		if err := session.Verify(mainPaths, localPaths, before); err == nil {
			t.Fatal("expected mutation rejection")
		}
	})

	t.Run("detects identical directory replacement", func(t *testing.T) {
		app, lib := setup(t)

		session, err := OpenPinnedLocalModuleSessionV4(app, "../lib")
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		before, err := session.Read(mainPaths, localPaths)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Rename(lib, lib+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lib, 0700); err != nil {
			t.Fatal(err)
		}

		for name, data := range map[string]string{
			"go.mod": "module example.com/lib\n",
			"lib.go": "package lib\n",
		} {
			if err := os.WriteFile(filepath.Join(lib, name), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}

		if err := session.Verify(mainPaths, localPaths, before); err == nil {
			t.Fatal("expected directory identity rejection")
		}
	})

	t.Run("rejects operations after close", func(t *testing.T) {
		app, _ := setup(t)

		session, err := OpenPinnedLocalModuleSessionV4(app, "../lib")
		if err != nil {
			t.Fatal(err)
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}

		if _, err := session.Read(mainPaths, localPaths); err == nil {
			t.Fatal("expected read rejection after close")
		}
		if err := session.Verify(mainPaths, localPaths, nil); err == nil {
			t.Fatal("expected verify rejection after close")
		}
	})
}
