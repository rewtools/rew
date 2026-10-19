package capsule

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifiedCapsuleV4RoundTrip(t *testing.T) {
	manifest := validLocalReplaceManifestV4()
	files := validWorkspaceWriterFilesV4()

	var archive bytes.Buffer
	if err := WriteWorkspaceV4(&archive, manifest, files); err != nil {
		t.Fatalf("write v4 capsule: %v", err)
	}

	filename := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	verified, err := ReadVerifiedV4(filename)
	if err != nil {
		t.Fatalf("read verified v4 capsule: %v", err)
	}

	if verified.Manifest.SchemaVersion != SchemaVersionV4 {
		t.Fatalf(
			"schema version = %d; want %d",
			verified.Manifest.SchemaVersion,
			SchemaVersionV4,
		)
	}

	if err := verified.Manifest.Validate(); err != nil {
		t.Fatalf("invalid verified manifest: %v", err)
	}

	if len(verified.Files) != len(files) {
		t.Fatalf(
			"verified file count = %d; want %d",
			len(verified.Files), len(files),
		)
	}

	for i, want := range files {
		got := verified.Files[i]

		if got.Path != want.Path ||
			!bytes.Equal(got.Data, want.Data) ||
			got.Mode.Perm() != want.Mode.Perm() {
			t.Fatalf(
				"verified file mismatch at index %d: got %q, want %q",
				i, got.Path, want.Path,
			)
		}
	}

	if len(verified.Manifest.LocalModules) != 1 {
		t.Fatalf(
			"local module count = %d; want 1",
			len(verified.Manifest.LocalModules),
		)
	}

	local := verified.Manifest.LocalModules[0]
	if local.ModulePath != "example.com/local-lib" ||
		local.SnapshotPath != "lib" ||
		len(local.Files) != 2 {
		t.Fatalf("unexpected local module metadata: %+v", local)
	}
}
