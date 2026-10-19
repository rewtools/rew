package capsule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifiedV4FromFileRejectsNil(t *testing.T) {
	if _, err := ReadVerifiedV4FromFile(nil); err == nil {
		t.Fatal("local reader accepted nil descriptor")
	}

	if _, err := ReadVerifiedToStoreV4FromFile(nil); err == nil {
		t.Fatal("mixed reader accepted nil descriptor")
	}
}

func TestVerifiedV4FromFileRejectsInvalidArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.rew")
	if err := os.WriteFile(
		path,
		[]byte(strings.Repeat("x", 2048)),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := ReadVerifiedV4FromFile(f); err == nil {
		t.Fatal("local reader accepted invalid archive")
	}

	if _, err := ReadVerifiedToStoreV4FromFile(f); err == nil {
		t.Fatal("mixed reader accepted invalid archive")
	}

	if _, err := f.Stat(); err != nil {
		t.Fatalf("reader unexpectedly closed caller descriptor: %v", err)
	}
}

func TestMixedVerifiedV4FromFilePinnedAfterPathReplacement(
	t *testing.T,
) {
	originalPath := mixedStoredArchiveFixtureV4(t)

	f, err := os.Open(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Replace the directory entry after opening the original file.
	// The descriptor must continue to reference the original archive.
	replacementPath := filepath.Join(
		filepath.Dir(originalPath), "replacement.rew",
	)

	if err := os.WriteFile(
		replacementPath,
		[]byte("replacement is not a valid capsule"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(
		replacementPath, originalPath,
	); err != nil {
		t.Fatal(err)
	}

	// Start from a nonzero offset to exercise rewind semantics.
	if _, err := f.Seek(512, 0); err != nil {
		t.Fatal(err)
	}

	verified, err := ReadVerifiedToStoreV4FromFile(f)
	if err != nil {
		t.Fatalf(
			"pinned original archive verification failed: %v",
			err,
		)
	}
	defer func() {
		if err := verified.Close(); err != nil {
			t.Errorf("close verified artifact store: %v", err)
		}
	}()

	if len(verified.Manifest.GoModules) != 1 {
		t.Fatalf(
			"external modules=%d, want 1",
			len(verified.Manifest.GoModules),
		)
	}

	if _, err := ReadVerifiedToStoreV4(originalPath); err == nil {
		t.Fatal("replacement path unexpectedly passed verification")
	}

	if _, err := f.Stat(); err != nil {
		t.Fatalf("caller descriptor was closed: %v", err)
	}
}
