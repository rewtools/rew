package capsule

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func verifiedRestoreFixtureV4(t *testing.T) VerifiedCapsuleV4 {
	t.Helper()

	var archive bytes.Buffer
	if err := WriteWorkspaceV4(
		&archive,
		validLocalReplaceManifestV4(),
		validWorkspaceWriterFilesV4(),
	); err != nil {
		t.Fatal(err)
	}

	filename := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	verified, err := ReadVerifiedV4(filename)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func TestRestoreWorkspaceV4RejectsModifiedBytesBeforeWrite(t *testing.T) {
	verified := verifiedRestoreFixtureV4(t)
	verified.Files[1].Data = []byte("tampered source\n")

	destination := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(destination, verified); err == nil {
		t.Fatal("accepted modified workspace bytes")
	}

	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid workspace created %d entries", len(entries))
	}
}

func TestRestoreWorkspaceV4RejectsNonemptyDestination(t *testing.T) {
	verified := verifiedRestoreFixtureV4(t)

	destination := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}

	sentinel := filepath.Join(destination, "important.txt")
	original := []byte("do not overwrite\n")
	if err := os.WriteFile(sentinel, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(destination, verified); err == nil {
		t.Fatal("accepted nonempty destination")
	}

	actual, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatal("existing file was modified")
	}

	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("destination changed: %d entries", len(entries))
	}
}

func TestRestoreWorkspaceV4RejectsSymlinkDestination(t *testing.T) {
	verified := verifiedRestoreFixtureV4(t)

	base := t.TempDir()
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "workspace")

	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(link, verified); err == nil {
		t.Fatal("accepted symlink destination")
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("symlink target was modified")
	}
}

func TestRestoreWorkspaceV4RejectsSecondRestore(t *testing.T) {
	verified := verifiedRestoreFixtureV4(t)

	destination := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(destination, verified); err != nil {
		t.Fatalf("first restore: %v", err)
	}

	mainPath := filepath.Join(destination, "app", "go.mod")
	before, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := RestoreWorkspaceV4(destination, verified); err == nil {
		t.Fatal("accepted second restore into nonempty destination")
	}

	after, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("second restore modified existing data")
	}
}
