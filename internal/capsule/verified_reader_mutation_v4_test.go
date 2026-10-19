package capsule

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMixedV4ValidCapsuleRejectsInPlaceWorkspaceMutation(
	t *testing.T,
) {
	path := mixedStoredArchiveFixtureV4(t)

	// Prove the starting archive is complete and valid.
	baseline, err := ReadVerifiedToStoreV4(path)
	if err != nil {
		t.Fatalf("valid baseline rejected: %v", err)
	}

	var target []byte
	for _, file := range baseline.Files {
		if len(file.Data) > len(target) {
			target = append([]byte(nil), file.Data...)
		}
	}

	if err := baseline.Close(); err != nil {
		t.Fatal(err)
	}
	if len(target) == 0 {
		t.Fatal("fixture lacks nonempty workspace data")
	}

	archive, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Locate unique workspace data, not TAR metadata.
	offset := bytes.Index(archive, target)
	if offset < 0 {
		t.Fatal("workspace content missing from archive")
	}
	if bytes.Index(archive[offset+1:], target) >= 0 {
		t.Fatal("workspace content is not unique")
	}

	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	before, err := reader.Stat()
	if err != nil {
		t.Fatal(err)
	}

	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}

	changed := target[0] ^ 0x01
	_, writeErr := writer.WriteAt(
		[]byte{changed}, int64(offset),
	)
	closeErr := writer.Close()

	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	after, err := reader.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("mutation changed inode")
	}
	if before.Size() != after.Size() {
		t.Fatal("mutation changed archive size")
	}

	// A valid manifest still describes the original workspace.
	// Verification must reject the altered payload.
	verified, err := ReadVerifiedToStoreV4FromFile(reader)
	if err == nil {
		_ = verified.Close()
		t.Fatal("tampered workspace accepted")
	}

	if !strings.Contains(
		err.Error(), "verify mixed v4 workspace",
	) {
		t.Fatalf("unexpected rejection stage: %v", err)
	}

	if _, statErr := reader.Stat(); statErr != nil {
		t.Fatalf("caller descriptor closed: %v", statErr)
	}

	t.Logf(
		"valid mixed capsule: one-byte workspace mutation rejected: %v",
		err,
	)
}
