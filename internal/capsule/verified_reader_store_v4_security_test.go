package capsule

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mixedTarEntryV4 struct {
	header tar.Header
	data   []byte
}

func readMixedTarEntriesV4(
	t *testing.T,
	archive []byte,
) []mixedTarEntryV4 {
	t.Helper()

	tr := tar.NewReader(bytes.NewReader(archive))
	var entries []mixedTarEntryV4

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}

		entries = append(entries, mixedTarEntryV4{
			header: *header,
			data:   data,
		})
	}

	return entries
}

func writeMixedTarEntriesV4(
	t *testing.T,
	entries []mixedTarEntryV4,
) []byte {
	t.Helper()

	var output bytes.Buffer
	tw := tar.NewWriter(&output)

	for _, entry := range entries {
		header := entry.header
		header.Size = int64(len(entry.data))

		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}

		if _, err := tw.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return output.Bytes()
}

func TestReadVerifiedToStoreV4RejectsTamperedArchive(
	t *testing.T,
) {
	originalFile := mixedStoredArchiveFixtureV4(t)

	original, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(t *testing.T, entries *[]mixedTarEntryV4)
	}{
		{
			name: "corrupted dependency ZIP",
			mutate: func(t *testing.T, entries *[]mixedTarEntryV4) {
				for i := range *entries {
					entry := &(*entries)[i]

					if entry.header.Name !=
						"dependencies/module-0001.zip" {
						continue
					}

					if len(entry.data) == 0 {
						t.Fatal("empty ZIP fixture")
					}

					entry.data[0] ^= 0xff
					return
				}
				t.Fatal("ZIP entry not found")
			},
		},
		{
			name: "corrupted workspace content",
			mutate: func(t *testing.T, entries *[]mixedTarEntryV4) {
				for i := range *entries {
					entry := &(*entries)[i]

					if entry.header.Name !=
						"workspace/app/main.go" {
						continue
					}

					if len(entry.data) == 0 {
						t.Fatal("empty workspace fixture")
					}

					entry.data[0] ^= 0xff
					return
				}
				t.Fatal("workspace entry not found")
			},
		},
		{
			name: "swapped dependency entries",
			mutate: func(t *testing.T, entries *[]mixedTarEntryV4) {
				found := -1

				for i := range *entries {
					if (*entries)[i].header.Name ==
						"dependencies/module-0001.zip" {
						found = i
						break
					}
				}

				if found < 0 || found+1 >= len(*entries) {
					t.Fatal("insufficient dependency entries")
				}

				(*entries)[found], (*entries)[found+1] =
					(*entries)[found+1], (*entries)[found]
			},
		},
		{
			name: "missing dependency info",
			mutate: func(t *testing.T, entries *[]mixedTarEntryV4) {
				for i := range *entries {
					if (*entries)[i].header.Name ==
						"dependencies/module-0001.info" {
						*entries = append(
							(*entries)[:i],
							(*entries)[i+1:]...,
						)
						return
					}
				}
				t.Fatal("info entry not found")
			},
		},
		{
			name: "unexpected extra dependency",
			mutate: func(t *testing.T, entries *[]mixedTarEntryV4) {
				*entries = append(*entries, mixedTarEntryV4{
					header: tar.Header{
						Name:     "dependencies/unexpected.txt",
						Mode:     0600,
						Typeflag: tar.TypeReg,
					},
					data: []byte("extra"),
				})
			},
		},
		{
			name: "renamed dependency artifact",
			mutate: func(t *testing.T, entries *[]mixedTarEntryV4) {
				for i := range *entries {
					if (*entries)[i].header.Name ==
						"dependencies/module-0001.mod" {
						(*entries)[i].header.Name =
							"dependencies/renamed-0001.mod"
						return
					}
				}
				t.Fatal("mod entry not found")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries := readMixedTarEntriesV4(t, original)
			tc.mutate(t, &entries)

			tampered := writeMixedTarEntriesV4(t, entries)

			filename := filepath.Join(
				t.TempDir(), "tampered.rew",
			)

			if err := os.WriteFile(
				filename, tampered, 0600,
			); err != nil {
				t.Fatal(err)
			}

			verified, err := ReadVerifiedToStoreV4(filename)

			if err == nil {
				_ = verified.Close()
				t.Fatal("SECURITY FAILURE: tampered archive accepted")
			}

			if verified.Store != nil {
				_ = verified.Close()
				t.Fatal(
					"SECURITY FAILURE: rejected archive returned a store",
				)
			}

			t.Log(fmt.Sprintf("correctly rejected: %v", err))
		})
	}
}

func TestReadVerifiedToStoreV4RejectsCorruptTarTerminator(
	t *testing.T,
) {
	originalFile := mixedStoredArchiveFixtureV4(t)

	original, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatal(err)
	}

	if len(original) < 1024 {
		t.Fatal("fixture too small for TAR terminator")
	}

	for _, tc := range []struct {
		name   string
		offset int
	}{
		{
			name:   "first terminator block",
			offset: len(original) - 1024,
		},
		{
			name:   "last terminator block",
			offset: len(original) - 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := bytes.Clone(original)

			if archive[tc.offset] != 0 {
				t.Fatalf(
					"fixture terminator byte already nonzero: %d",
					archive[tc.offset],
				)
			}

			archive[tc.offset] = 0x7f

			filename := filepath.Join(
				t.TempDir(),
				"corrupt-terminator.rew",
			)

			if err := os.WriteFile(
				filename, archive, 0600,
			); err != nil {
				t.Fatal(err)
			}

			verified, err := ReadVerifiedToStoreV4(filename)
			if err == nil {
				_ = verified.Close()
				t.Fatal(
					"SECURITY FAILURE: nonzero TAR terminator accepted",
				)
			}

			if verified.Store != nil {
				_ = verified.Close()
				t.Fatal(
					"SECURITY FAILURE: rejected TAR returned store",
				)
			}

			t.Logf("rejected corrupt terminator: %v", err)
		})
	}
}

func TestReadVerifiedToStoreV4RejectsTruncationAndManifestDamage(
	t *testing.T,
) {
	filename := mixedStoredArchiveFixtureV4(t)

	original, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if len(original) <= 2048 {
		t.Fatal("fixture unexpectedly small")
	}

	cases := []struct {
		name   string
		damage func(*testing.T) []byte
	}{
		{
			name: "truncate last byte",
			damage: func(t *testing.T) []byte {
				return bytes.Clone(original[:len(original)-1])
			},
		},
		{
			name: "truncate last TAR block",
			damage: func(t *testing.T) []byte {
				return bytes.Clone(original[:len(original)-512])
			},
		},
		{
			name: "truncate both TAR terminator blocks",
			damage: func(t *testing.T) []byte {
				return bytes.Clone(original[:len(original)-1024])
			},
		},
		{
			name: "invalid manifest JSON same archive length",
			damage: func(t *testing.T) []byte {
				entries := readMixedTarEntriesV4(t, original)

				if len(entries) == 0 ||
					entries[0].header.Name != "manifest.json" ||
					len(entries[0].data) == 0 {
					t.Fatal("invalid fixture manifest")
				}

				entries[0].data[0] = 0xff

				out := writeMixedTarEntriesV4(t, entries)
				if len(out) != len(original) {
					t.Fatalf(
						"manifest mutation changed archive size: %d -> %d",
						len(original), len(out),
					)
				}

				return out
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			damaged := tc.damage(t)

			damagedFile := filepath.Join(
				t.TempDir(), "damaged.rew",
			)
			if err := os.WriteFile(
				damagedFile, damaged, 0600,
			); err != nil {
				t.Fatal(err)
			}

			verified, err := ReadVerifiedToStoreV4(damagedFile)
			if err == nil {
				_ = verified.Close()
				t.Fatal("SECURITY FAILURE: damaged archive accepted")
			}

			if verified.Store != nil {
				_ = verified.Close()
				t.Fatal(
					"SECURITY FAILURE: failed read returned artifact store",
				)
			}

			t.Logf("correctly rejected: %v", err)
		})
	}
}

func TestReadVerifiedToStoreV4CleansStoreOnLateFailure(
	t *testing.T,
) {
	originalFile := mixedStoredArchiveFixtureV4(t)

	original, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatal(err)
	}

	entries := readMixedTarEntriesV4(t, original)

	if len(entries) == 0 {
		t.Fatal("empty archive fixture")
	}

	last := &entries[len(entries)-1]

	if last.header.Name != "dependencies/module-0001.info" {
		t.Fatalf(
			"expected final .info artifact, got %q",
			last.header.Name,
		)
	}

	if len(last.data) == 0 {
		t.Fatal("empty .info artifact")
	}

	// Damage only the final artifact, preserving the TAR size.
	// Earlier .zip and .mod entries must pass store.Put.
	last.data[0] ^= 0xff

	damaged := writeMixedTarEntriesV4(t, entries)

	if len(damaged) != len(original) {
		t.Fatalf(
			"archive length changed: %d -> %d",
			len(original), len(damaged),
		)
	}

	filename := filepath.Join(t.TempDir(), "late-failure.rew")

	if err := os.WriteFile(filename, damaged, 0600); err != nil {
		t.Fatal(err)
	}

	// ArtifactStoreV3 uses os.MkdirTemp("", ...).
	// Isolate its temporary root to observe cleanup directly.
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	verified, err := ReadVerifiedToStoreV4(filename)

	if err == nil {
		_ = verified.Close()
		t.Fatal("SECURITY FAILURE: corrupted final artifact accepted")
	}

	if verified.Store != nil {
		_ = verified.Close()
		t.Fatal(
			"SECURITY FAILURE: failed read returned an open store",
		)
	}

	if !strings.Contains(err.Error(), "artifact SHA-256 mismatch") {
		t.Fatalf(
			"expected late artifact verification error, got: %v",
			err,
		)
	}

	remaining, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(remaining) != 0 {
		for _, entry := range remaining {
			t.Logf("leftover temporary entry: %s", entry.Name())
		}
		t.Fatal(
			"SECURITY FAILURE: artifact store was not cleaned",
		)
	}

	t.Log("late artifact verification rejected; temporary store removed")
}
