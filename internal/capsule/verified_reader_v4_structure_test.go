package capsule

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadVerifiedV4RejectsSameSizeStructuralTampering(t *testing.T) {
	var original bytes.Buffer

	if err := WriteWorkspaceV4(
		&original,
		validLocalReplaceManifestV4(),
		validWorkspaceWriterFilesV4(),
	); err != nil {
		t.Fatal(err)
	}

	originalSize := original.Len()

	tests := []struct {
		name   string
		mutate func([]v4SecurityEntry) ([]byte, error)
	}{
		{
			name: "symlink with original archive size",
			mutate: func(entries []v4SecurityEntry) ([]byte, error) {
				found := false

				for i := range entries {
					if entries[i].header.Name != "workspace/lib/lib.go" {
						continue
					}

					entries[i].header.Typeflag = tar.TypeSymlink
					entries[i].header.Linkname = "../../etc/passwd"
					entries[i].header.Size = 0
					entries[i].data = nil
					found = true
					break
				}

				if !found {
					return nil, fmt.Errorf("target entry not found")
				}

				data, err := writeV4SecurityEntries(entries)
				if err != nil {
					return nil, err
				}
				if len(data) > originalSize {
					return nil, fmt.Errorf("symlink archive grew")
				}

				// Preserve the source archive's length. This cannot
				// be rejected solely by the file-size preflight.
				return append(data, make([]byte, originalSize-len(data))...), nil
			},
		},
		{
			name: "extra entry with original archive size",
			mutate: func(entries []v4SecurityEntry) ([]byte, error) {
				content := []byte("unexpected\n")

				entries = append(entries, v4SecurityEntry{
					header: tar.Header{
						Name:     "workspace/lib/extra.go",
						Mode:     0600,
						Typeflag: tar.TypeReg,
						Size:     int64(len(content)),
					},
					data: content,
				})

				data, err := writeV4SecurityEntries(entries)
				if err != nil {
					return nil, err
				}
				if len(data) < originalSize {
					return nil, fmt.Errorf("extra-entry archive shrank")
				}

				// Remove trailing TAR blocks while retaining the
				// unexpected entry. Its size now matches the source.
				return data[:originalSize], nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := readV4SecurityEntries(original.Bytes())
			if err != nil {
				t.Fatal(err)
			}

			modified, err := tc.mutate(entries)
			if err != nil {
				t.Fatal(err)
			}

			if len(modified) != originalSize {
				t.Fatalf(
					"modified archive size = %d; want %d",
					len(modified), originalSize,
				)
			}

			filename := filepath.Join(t.TempDir(), "modified.rew")
			if err := os.WriteFile(filename, modified, 0600); err != nil {
				t.Fatal(err)
			}

			_, err = ReadVerifiedV4(filename)
			if err == nil {
				t.Fatal("accepted structurally modified TAR")
			}

			if strings.Contains(err.Error(), "archive size mismatch") {
				t.Fatalf(
					"rejected by size instead of TAR structure: %v",
					err,
				)
			}

			t.Logf("structural rejection: %v", err)
		})
	}
}
