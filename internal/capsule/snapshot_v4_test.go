package capsule

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteWorkspaceSnapshotV4TwoModules(t *testing.T) {
	var archive bytes.Buffer

	if err := WriteWorkspaceV4(
		&archive,
		validLocalReplaceManifestV4(),
		validWorkspaceWriterFilesV4(),
	); err != nil {
		t.Fatalf("create V4 capsule: %v", err)
	}

	filename := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	verified, err := ReadVerifiedV4(filename)
	if err != nil {
		t.Fatalf("verify V4 capsule: %v", err)
	}

	var snapshot bytes.Buffer
	if err := WriteWorkspaceSnapshotV4(&snapshot, verified); err != nil {
		t.Fatalf("write Docker snapshot: %v", err)
	}

	reader := tar.NewReader(bytes.NewReader(snapshot.Bytes()))
	expectedFiles := validWorkspaceWriterFilesV4()

	for i, expected := range expectedFiles {
		header, err := reader.Next()
		if err != nil {
			t.Fatalf("read snapshot entry %d: %v", i, err)
		}

		if header.Name != expected.Path {
			t.Fatalf(
				"snapshot path %d = %q, want %q",
				i, header.Name, expected.Path,
			)
		}
		if header.Typeflag != tar.TypeReg {
			t.Fatalf(
				"snapshot entry %q is not regular",
				header.Name,
			)
		}
		if header.Mode != 0644 {
			t.Fatalf(
				"snapshot mode %q = %o, want 0644",
				header.Name, header.Mode,
			)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, expected.Data) {
			t.Fatalf("snapshot data mismatch: %q", expected.Path)
		}
	}

	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("unexpected extra snapshot entry: %v", err)
	}
}
