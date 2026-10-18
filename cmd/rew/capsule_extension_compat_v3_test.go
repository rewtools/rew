package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCapsuleV3ExtensionCompatibility(t *testing.T) {
	legacyPath := makeAuditCapsuleV3(t)

	original, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}

	newPath := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(newPath, original, 0600); err != nil {
		t.Fatal(err)
	}

	expectedHash := sha256.Sum256(original)

	for _, path := range []string{legacyPath, newPath} {
		t.Run(filepath.Ext(path), func(t *testing.T) {
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if sha256.Sum256(before) != expectedHash {
				t.Fatal("capsule SHA-256 differs from original")
			}

			verified, err := capsule.ReadVerifiedV3(path)
			if err != nil {
				t.Fatalf("verified reader failed: %v", err)
			}

			if verified.Manifest.SchemaVersion != capsule.SchemaVersionV3 {
				t.Fatal("unexpected capsule schema version")
			}

			code, output := captureAuditCLIOutput(t, []string{
				"audit", path,
			})

			if code != 0 {
				t.Fatalf("audit exit = %d:\n%s", code, output)
			}

			if !strings.Contains(output, "Capsule integrity: verified") {
				t.Fatalf("audit did not verify integrity:\n%s", output)
			}

			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(before, after) {
				t.Fatal("audit modified capsule bytes")
			}

			if sha256.Sum256(after) != expectedHash {
				t.Fatal("capsule SHA-256 changed after audit")
			}
		})
	}
}
