package capsule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedLocalModuleSessionV4(t *testing.T) {
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
			"app/go.mod": "module example.com/app\n",
			"lib/go.mod": "module example.com/lib\n",
			"lib/lib.go": "package lib\n",
		} {
			if err := os.WriteFile(
				filepath.Join(parent, name), []byte(data), 0600,
			); err != nil {
				t.Fatal(err)
			}
		}

		return app, lib
	}

	open := func(t *testing.T, app string) *PinnedLocalModuleSessionV4 {
		t.Helper()

		session, err := OpenPinnedLocalModuleSessionV4(app, "../lib")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := session.Close(); err != nil {
				t.Errorf("close pinned session: %v", err)
			}
		})
		return session
	}

	t.Run("unchanged roots remain valid", func(t *testing.T) {
		app, _ := setup(t)
		session := open(t, app)

		if err := session.VerifyRootIdentity(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects identical-content directory replacement", func(t *testing.T) {
		app, lib := setup(t)
		session := open(t, app)

		oldLib := lib + "-old"
		if err := os.Rename(lib, oldLib); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lib, 0700); err != nil {
			t.Fatal(err)
		}

		for name, data := range map[string]string{
			"go.mod": "module example.com/lib\n",
			"lib.go": "package lib\n",
		} {
			if err := os.WriteFile(
				filepath.Join(lib, name), []byte(data), 0600,
			); err != nil {
				t.Fatal(err)
			}
		}

		if err := session.VerifyRootIdentity(); err == nil {
			t.Fatal("expected root identity mismatch despite identical bytes")
		}
	})

	t.Run("rejects removed original directory entry", func(t *testing.T) {
		app, lib := setup(t)
		session := open(t, app)

		if err := os.Rename(lib, lib+"-old"); err != nil {
			t.Fatal(err)
		}

		if err := session.VerifyRootIdentity(); err == nil {
			t.Fatal("expected missing original root rejection")
		}
	})
}
