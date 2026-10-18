package capsule

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type v4SecurityEntry struct {
	header tar.Header
	data   []byte
}

func readV4SecurityEntries(data []byte) ([]v4SecurityEntry, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	var entries []v4SecurityEntry

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}

		entries = append(entries, v4SecurityEntry{
			header: *header,
			data:   content,
		})
	}
}

func writeV4SecurityEntries(entries []v4SecurityEntry) ([]byte, error) {
	var output bytes.Buffer
	tw := tar.NewWriter(&output)

	for _, entry := range entries {
		header := entry.header

		if err := tw.WriteHeader(&header); err != nil {
			return nil, err
		}
		if _, err := tw.Write(entry.data); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func TestReadVerifiedV4RejectsTamperedArchives(t *testing.T) {
	var original bytes.Buffer

	if err := WriteWorkspaceV4(
		&original,
		validLocalReplaceManifestV4(),
		validWorkspaceWriterFilesV4(),
	); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func([]byte) ([]byte, error)
	}{
		{
			name: "modified source bytes",
			mutate: func(data []byte) ([]byte, error) {
				entries, err := readV4SecurityEntries(data)
				if err != nil {
					return nil, err
				}

				for i := range entries {
					if entries[i].header.Name == "workspace/lib/lib.go" {
						if len(entries[i].data) == 0 {
							return nil, fmt.Errorf("empty target file")
						}
						entries[i].data[0] ^= 0x01
						return writeV4SecurityEntries(entries)
					}
				}
				return nil, fmt.Errorf("source entry not found")
			},
		},
		{
			name: "wrong entry order",
			mutate: func(data []byte) ([]byte, error) {
				entries, err := readV4SecurityEntries(data)
				if err != nil {
					return nil, err
				}
				if len(entries) < 3 {
					return nil, fmt.Errorf("insufficient entries")
				}

				entries[1], entries[2] = entries[2], entries[1]
				return writeV4SecurityEntries(entries)
			},
		},
		{
			name: "workspace symlink",
			mutate: func(data []byte) ([]byte, error) {
				entries, err := readV4SecurityEntries(data)
				if err != nil {
					return nil, err
				}

				for i := range entries {
					if entries[i].header.Name == "workspace/lib/lib.go" {
						entries[i].header.Typeflag = tar.TypeSymlink
						entries[i].header.Linkname = "../../etc/passwd"
						entries[i].header.Size = 0
						entries[i].data = nil
						return writeV4SecurityEntries(entries)
					}
				}
				return nil, fmt.Errorf("source entry not found")
			},
		},
		{
			name: "extra workspace entry",
			mutate: func(data []byte) ([]byte, error) {
				entries, err := readV4SecurityEntries(data)
				if err != nil {
					return nil, err
				}

				content := []byte("unexpected\n")
				entries = append(entries, v4SecurityEntry{
					header: tar.Header{
						Name:     "workspace/lib/extra.go",
						Typeflag: tar.TypeReg,
						Mode:     0600,
						Size:     int64(len(content)),
					},
					data: content,
				})

				return writeV4SecurityEntries(entries)
			},
		},
		{
			name: "truncated archive",
			mutate: func(data []byte) ([]byte, error) {
				if len(data) < 2048 {
					return nil, fmt.Errorf("archive too small")
				}
				return append([]byte(nil), data[:len(data)-512]...), nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modified, err := tc.mutate(original.Bytes())
			if err != nil {
				t.Fatalf("prepare modified archive: %v", err)
			}

			filename := filepath.Join(t.TempDir(), "modified.rew")
			if err := os.WriteFile(filename, modified, 0600); err != nil {
				t.Fatal(err)
			}

			verified, err := ReadVerifiedV4(filename)
			if err == nil {
				t.Fatalf(
					"accepted modified archive: %+v",
					verified.Manifest,
				)
			}
			t.Logf("rejected: %v", err)
		})
	}
}
