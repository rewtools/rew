package capsule

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadVerifiedV4RejectsNonzeroTarTerminator(t *testing.T) {
	var archive bytes.Buffer

	if err := WriteWorkspaceV4(
		&archive,
		validLocalReplaceManifestV4(),
		validWorkspaceWriterFilesV4(),
	); err != nil {
		t.Fatal(err)
	}

	original := archive.Bytes()

	if len(original) < 1024 {
		t.Fatal("archive has no room for TAR terminator")
	}

	// The writer emits zero-filled TAR termination blocks.
	// Change only the last byte, keeping all headers, workspace
	// content, manifest metadata, and total archive length intact.
	if original[len(original)-1] != 0 {
		t.Fatal("expected zero-filled final archive byte")
	}

	mutated := append([]byte(nil), original...)
	mutated[len(mutated)-1] = 0x7f

	filename := filepath.Join(t.TempDir(), "bad-terminator.rew")
	if err := os.WriteFile(filename, mutated, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadVerifiedV4(filename); err == nil {
		t.Fatal("reader accepted nonzero TAR terminator byte")
	}
}
