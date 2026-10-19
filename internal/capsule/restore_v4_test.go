package capsule

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreWorkspaceV4RoundTrip(t *testing.T) {
	manifest := validLocalReplaceManifestV4()
	files := validWorkspaceWriterFilesV4()

	var archive bytes.Buffer
	if err := WriteWorkspaceV4(&archive, manifest, files); err != nil {
		t.Fatalf("write V4 archive: %v", err)
	}

	capsulePath := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(capsulePath, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	verified, err := ReadVerifiedV4(capsulePath)
	if err != nil {
		t.Fatalf("verify V4 archive: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(destination, verified); err != nil {
		t.Fatalf("restore V4 workspace: %v", err)
	}

	for _, expected := range files {
		restoredPath := filepath.Join(
			destination,
			filepath.FromSlash(expected.Path),
		)

		actual, err := os.ReadFile(restoredPath)
		if err != nil {
			t.Fatalf("read restored %s: %v", expected.Path, err)
		}
		if !bytes.Equal(actual, expected.Data) {
			t.Fatalf("restored bytes differ: %s", expected.Path)
		}

		info, err := os.Lstat(restoredPath)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("restored file not regular: %s", expected.Path)
		}
	}

	appMod, err := os.ReadFile(
		filepath.Join(destination, "app", "go.mod"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(
		appMod,
		[]byte("replace example.com/local-lib => ../lib"),
	) {
		t.Fatal("restored app/go.mod lost relative replacement")
	}

	localMod, err := os.ReadFile(
		filepath.Join(destination, "lib", "go.mod"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(
		localMod,
		[]byte("module example.com/local-lib"),
	) {
		t.Fatal("restored lib/go.mod has wrong module identity")
	}
}
