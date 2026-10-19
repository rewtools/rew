package capsule

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWorkspaceSnapshotFromStoreV4MixedRoundTrip(
	t *testing.T,
) {
	filename := mixedStoredArchiveFixtureV4(t)

	verified, err := ReadVerifiedToStoreV4(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := verified.Close(); err != nil {
			t.Errorf("close verified store: %v", err)
		}
	}()

	if len(verified.Manifest.GoModules) != 1 {
		t.Fatal("fixture must contain one external module")
	}
	if len(verified.Manifest.LocalModules) != 1 {
		t.Fatal("fixture must contain one local module")
	}
	if len(verified.Files) == 0 {
		t.Fatal("fixture workspace is empty")
	}

	var output bytes.Buffer

	if err := WriteWorkspaceSnapshotFromStoreV4(
		&output, verified,
	); err != nil {
		t.Fatalf("write mixed workspace snapshot: %v", err)
	}

	want := make(map[string][]byte, len(verified.Files))

	for _, file := range verified.Files {
		if _, exists := want[file.Path]; exists {
			t.Fatalf("duplicate fixture file %q", file.Path)
		}
		want[file.Path] = file.Data
	}

	reader := tar.NewReader(bytes.NewReader(output.Bytes()))
	seen := make(map[string]bool, len(want))

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read workspace TAR: %v", err)
		}

		if header.Typeflag != tar.TypeReg {
			t.Fatalf(
				"unexpected TAR entry type for %q: %d",
				header.Name,
				header.Typeflag,
			)
		}

		expectedData, ok := want[header.Name]
		if !ok {
			t.Fatalf(
				"unexpected workspace TAR entry %q",
				header.Name,
			)
		}
		if seen[header.Name] {
			t.Fatalf("duplicate TAR entry %q", header.Name)
		}
		seen[header.Name] = true

		if header.Size != int64(len(expectedData)) {
			t.Fatalf(
				"incorrect size for %q: got %d, want %d",
				header.Name,
				header.Size,
				len(expectedData),
			)
		}

		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read %q: %v", header.Name, err)
		}
		if !bytes.Equal(got, expectedData) {
			t.Fatalf(
				"workspace content mismatch for %q",
				header.Name,
			)
		}
	}

	if len(seen) != len(want) {
		t.Fatalf(
			"missing TAR entries: got %d, want %d",
			len(seen),
			len(want),
		)
	}
}

type failingMixedWorkspaceWriterV4 struct{}

func (failingMixedWorkspaceWriterV4) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestWriteWorkspaceSnapshotFromStoreV4Security(
	t *testing.T,
) {
	t.Run("workspace content mutation", func(t *testing.T) {
		filename := mixedStoredArchiveFixtureV4(t)

		verified, err := ReadVerifiedToStoreV4(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := verified.Close(); err != nil {
				t.Error(err)
			}
		}()

		if len(verified.Files) == 0 ||
			len(verified.Files[0].Data) == 0 {
			t.Fatal("empty workspace fixture")
		}

		verified.Files[0].Data[0] ^= 0xff

		var output bytes.Buffer
		err = WriteWorkspaceSnapshotFromStoreV4(
			&output, verified,
		)

		if err == nil {
			t.Fatal("SECURITY FAILURE: mutated workspace accepted")
		}
		if output.Len() != 0 {
			t.Fatalf("wrote %d bytes before rejection", output.Len())
		}
	})

	t.Run("local module identity mismatch", func(t *testing.T) {
		filename := mixedStoredArchiveFixtureV4(t)

		verified, err := ReadVerifiedToStoreV4(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := verified.Close(); err != nil {
				t.Error(err)
			}
		}()

		if len(verified.Manifest.LocalModules) != 1 {
			t.Fatal("invalid local module fixture")
		}

		local := verified.Manifest.LocalModules[0]
		target := local.SnapshotPath + "/go.mod"

		var changed []byte
		found := false

		for i := range verified.Files {
			f := &verified.Files[i]
			if f.Path != target {
				continue
			}

			old := "module " + local.ModulePath
			if !strings.Contains(string(f.Data), old) {
				t.Fatalf("missing module declaration %q", old)
			}

			changed = []byte(strings.Replace(
				string(f.Data), old,
				"module example.com/wrong-local", 1,
			))
			f.Data = changed
			found = true
			break
		}
		if !found {
			t.Fatalf("missing local go.mod %q", target)
		}

		digest := fmt.Sprintf(
			"sha256:%x", sha256.Sum256(changed),
		)

		found = false
		for i := range verified.Manifest.Workspace.Files {
			f := &verified.Manifest.Workspace.Files[i]
			if f.Path == target {
				f.Size = int64(len(changed))
				f.SHA256 = digest
				found = true
			}
		}
		if !found {
			t.Fatal("missing workspace metadata")
		}

		found = false
		for i := range verified.Manifest.LocalModules[0].Files {
			f := &verified.Manifest.LocalModules[0].Files[i]
			if f.Path == "go.mod" {
				f.Size = int64(len(changed))
				f.SHA256 = digest
				found = true
			}
		}
		if !found {
			t.Fatal("missing local module metadata")
		}

		var output bytes.Buffer
		err = WriteWorkspaceSnapshotFromStoreV4(
			&output, verified,
		)

		if err == nil {
			t.Fatal("SECURITY FAILURE: wrong local module accepted")
		}
		if output.Len() != 0 {
			t.Fatalf("wrote %d bytes before rejection", output.Len())
		}
		if !strings.Contains(err.Error(), "identity mismatch") {
			t.Fatalf("wrong rejection reason: %v", err)
		}
	})

	t.Run("output failure", func(t *testing.T) {
		filename := mixedStoredArchiveFixtureV4(t)

		verified, err := ReadVerifiedToStoreV4(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := verified.Close(); err != nil {
				t.Error(err)
			}
		}()

		err = WriteWorkspaceSnapshotFromStoreV4(
			failingMixedWorkspaceWriterV4{},
			verified,
		)
		if err == nil {
			t.Fatal("SECURITY FAILURE: destination error ignored")
		}
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("destination error not preserved: %v", err)
		}
	})
}

func TestWriteWorkspaceSnapshotFromStoreV4ArtifactTampering(
	t *testing.T,
) {
	cases := []struct {
		name   string
		mutate func(string, []byte) error
	}{
		{
			name: "replaced inode",
			mutate: func(zipPath string, original []byte) error {
				replacement := filepath.Join(
					filepath.Dir(zipPath), ".replacement-module-zip",
				)
				changed := bytes.Clone(original)
				changed[0] ^= 0xff

				if err := os.WriteFile(
					replacement, changed, 0600,
				); err != nil {
					return err
				}
				return os.Rename(replacement, zipPath)
			},
		},
		{
			name: "in-place mutation",
			mutate: func(zipPath string, original []byte) error {
				f, err := os.OpenFile(zipPath, os.O_RDWR, 0)
				if err != nil {
					return err
				}

				changed := []byte{original[0] ^ 0xff}
				_, writeErr := f.WriteAt(changed, 0)
				closeErr := f.Close()

				if writeErr != nil {
					return writeErr
				}
				return closeErr
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filename := mixedStoredArchiveFixtureV4(t)

			verified, err := ReadVerifiedToStoreV4(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := verified.Close(); err != nil {
					t.Error(err)
				}
			}()

			f, err := verified.Store.Open("module-0001.zip")
			if err != nil {
				t.Fatal(err)
			}
			zipPath := f.Name()

			original, readErr := io.ReadAll(f)
			before, statErr := f.Stat()
			closeErr := f.Close()

			if readErr != nil || statErr != nil || closeErr != nil {
				t.Fatalf(
					"inspect ZIP: %v, %v, %v",
					readErr, statErr, closeErr,
				)
			}
			if len(original) == 0 {
				t.Fatal("empty external ZIP fixture")
			}

			if err := tc.mutate(zipPath, original); err != nil {
				t.Fatal(err)
			}

			after, err := os.Stat(zipPath)
			if err != nil {
				t.Fatal(err)
			}

			if before.Size() != after.Size() {
				t.Fatal("test mutation changed file size")
			}

			sameInode := os.SameFile(before, after)
			if tc.name == "replaced inode" && sameInode {
				t.Fatal("test failed to replace inode")
			}
			if tc.name == "in-place mutation" && !sameInode {
				t.Fatal("test unexpectedly replaced inode")
			}

			var output bytes.Buffer

			err = WriteWorkspaceSnapshotFromStoreV4(
				&output, verified,
			)
			if err == nil {
				t.Fatal(
					"SECURITY FAILURE: tampered external artifact accepted",
				)
			}
			if output.Len() != 0 {
				t.Fatalf(
					"SECURITY FAILURE: emitted %d bytes before rejection",
					output.Len(),
				)
			}

			t.Logf("artifact tampering rejected: %v", err)
		})
	}
}

func TestWriteWorkspaceSnapshotFromStoreV4Consistency(
	t *testing.T,
) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *VerifiedStoredCapsuleV4)
	}{
		{
			name: "missing workspace file",
			mutate: func(t *testing.T, v *VerifiedStoredCapsuleV4) {
				t.Helper()
				if len(v.Files) < 2 {
					t.Fatal("insufficient fixture files")
				}
				v.Files = v.Files[:len(v.Files)-1]
			},
		},
		{
			name: "duplicate workspace path",
			mutate: func(t *testing.T, v *VerifiedStoredCapsuleV4) {
				t.Helper()
				if len(v.Files) < 2 {
					t.Fatal("insufficient fixture files")
				}
				v.Files[1].Path = v.Files[0].Path
			},
		},
		{
			name: "unexpected workspace path",
			mutate: func(t *testing.T, v *VerifiedStoredCapsuleV4) {
				t.Helper()
				if len(v.Files) == 0 {
					t.Fatal("empty fixture")
				}
				v.Files[0].Path = "app/unexpected.go"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filename := mixedStoredArchiveFixtureV4(t)

			verified, err := ReadVerifiedToStoreV4(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := verified.Close(); err != nil {
					t.Error(err)
				}
			}()

			tc.mutate(t, &verified)

			var output bytes.Buffer
			err = WriteWorkspaceSnapshotFromStoreV4(
				&output, verified,
			)

			if err == nil {
				t.Fatal("SECURITY FAILURE: inconsistent workspace accepted")
			}
			if output.Len() != 0 {
				t.Fatalf(
					"SECURITY FAILURE: %d bytes emitted",
					output.Len(),
				)
			}
			t.Logf("correctly rejected: %v", err)
		})
	}
}
