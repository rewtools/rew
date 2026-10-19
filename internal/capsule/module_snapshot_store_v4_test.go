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

	"golang.org/x/mod/module"
)

func TestWriteGoModuleSnapshotFromStoreV4MixedRoundTrip(
	t *testing.T,
) {
	filename := mixedStoredArchiveFixtureV4(t)

	verified, err := ReadVerifiedToStoreV4(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := verified.Close(); err != nil {
			t.Errorf("close verified capsule: %v", err)
		}
	}()

	if len(verified.Manifest.GoModules) != 1 {
		t.Fatal("expected exactly one external module")
	}

	mod := verified.Manifest.GoModules[0]

	escapedPath, err := module.EscapePath(mod.Path)
	if err != nil {
		t.Fatal(err)
	}

	escapedVersion, err := module.EscapeVersion(mod.Version)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	if err := WriteGoModuleSnapshotFromStoreV4(
		&output, verified,
	); err != nil {
		t.Fatalf("create v4 module snapshot: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(output.Bytes()))

	for _, ext := range []string{
		".zip", ".mod", ".info",
	} {
		source := "module-0001" + ext

		wantName := "cache/download/" +
			escapedPath + "/@v/" +
			escapedVersion + ext

		header, err := tr.Next()
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}

		if header.Name != wantName {
			t.Fatalf(
				"snapshot name = %q, want %q",
				header.Name, wantName,
			)
		}

		if header.Typeflag != tar.TypeReg {
			t.Fatalf(
				"snapshot entry %q is not regular",
				header.Name,
			)
		}

		got, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}

		stored, err := verified.Store.Open(source)
		if err != nil {
			t.Fatal(err)
		}

		want, readErr := io.ReadAll(stored)
		closeErr := stored.Close()

		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}

		if !bytes.Equal(got, want) {
			t.Fatalf(
				"snapshot artifact %q differs from verified store",
				source,
			)
		}
	}

	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf(
			"unexpected extra module snapshot entry: %v",
			err,
		)
	}
}

type failingModuleSnapshotWriterV4 struct{}

func (failingModuleSnapshotWriterV4) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestWriteGoModuleSnapshotFromStoreV4Security(t *testing.T) {
	t.Run("altered external artifact digest", func(t *testing.T) {
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

		if len(verified.Manifest.GoModules) != 1 ||
			len(verified.Manifest.GoModules[0].Artifacts) == 0 {
			t.Fatal("unexpected external artifact fixture")
		}

		artifact := &verified.Manifest.GoModules[0].Artifacts[0]

		if !strings.HasPrefix(artifact.SHA256, "sha256:") {
			t.Fatal("unexpected digest format")
		}

		// Keep the digest syntactically valid but make it incorrect.
		artifact.SHA256 = "sha256:" + strings.Repeat("0", 64)

		var output bytes.Buffer
		err = WriteGoModuleSnapshotFromStoreV4(&output, verified)

		if err == nil {
			t.Fatal("SECURITY FAILURE: incorrect digest accepted")
		}
		if output.Len() != 0 {
			t.Fatalf(
				"SECURITY FAILURE: %d bytes written before verification",
				output.Len(),
			)
		}
	})

	t.Run("mutated verified workspace", func(t *testing.T) {
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
			t.Fatal("unexpected workspace fixture")
		}

		verified.Files[0].Data[0] ^= 0xff

		var output bytes.Buffer
		err = WriteGoModuleSnapshotFromStoreV4(&output, verified)

		if err == nil {
			t.Fatal("SECURITY FAILURE: mutated workspace accepted")
		}
		if output.Len() != 0 {
			t.Fatalf(
				"SECURITY FAILURE: %d bytes written before rejection",
				output.Len(),
			)
		}
	})

	t.Run("closed artifact store", func(t *testing.T) {
		filename := mixedStoredArchiveFixtureV4(t)

		verified, err := ReadVerifiedToStoreV4(filename)
		if err != nil {
			t.Fatal(err)
		}
		if err := verified.Close(); err != nil {
			t.Fatal(err)
		}

		var output bytes.Buffer
		err = WriteGoModuleSnapshotFromStoreV4(&output, verified)

		if err == nil {
			t.Fatal("SECURITY FAILURE: closed artifact store accepted")
		}
		if output.Len() != 0 {
			t.Fatalf(
				"SECURITY FAILURE: %d bytes written with closed store",
				output.Len(),
			)
		}
	})

	t.Run("destination write failure", func(t *testing.T) {
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

		err = WriteGoModuleSnapshotFromStoreV4(
			failingModuleSnapshotWriterV4{},
			verified,
		)

		if err == nil {
			t.Fatal("SECURITY FAILURE: destination error ignored")
		}
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf(
				"destination error not preserved: %v",
				err,
			)
		}
	})
}

// triggeredModuleSnapshotWriterV4 performs a deterministic mutation
// inside a TAR output Write call. No scheduling or timing assumptions.
type triggeredModuleSnapshotWriterV4 struct {
	output  bytes.Buffer
	trigger func([]byte) bool
	mutate  func() error
	called  bool
}

func (w *triggeredModuleSnapshotWriterV4) Write(p []byte) (int, error) {
	if !w.called && w.trigger(p) {
		w.called = true
		if err := w.mutate(); err != nil {
			return 0, err
		}
	}
	return w.output.Write(p)
}

func TestWriteGoModuleSnapshotFromStoreV4TOCTOU(t *testing.T) {
	t.Run("replaced inode after preflight", func(t *testing.T) {
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
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read original ZIP: %v, %v", readErr, closeErr)
		}

		destination := &triggeredModuleSnapshotWriterV4{
			trigger: func([]byte) bool { return true },
			mutate: func() error {
				replacement := filepath.Join(
					filepath.Dir(zipPath), ".replacement-zip",
				)

				changed := bytes.Clone(original)
				if len(changed) == 0 {
					return errors.New("empty ZIP fixture")
				}
				changed[0] ^= 0xff

				if err := os.WriteFile(
					replacement, changed, 0600,
				); err != nil {
					return err
				}
				return os.Rename(replacement, zipPath)
			},
		}

		err = WriteGoModuleSnapshotFromStoreV4(
			destination, verified,
		)

		if !destination.called {
			t.Fatalf(
				"mutation trigger not reached: %v", err,
			)
		}
		if err == nil {
			tr := tar.NewReader(
				bytes.NewReader(destination.output.Bytes()),
			)

			header, readErr := tr.Next()
			if readErr != nil {
				t.Fatalf("read successful TAR: %v", readErr)
			}

			got, readErr := io.ReadAll(tr)
			if readErr != nil {
				t.Fatalf("read successful ZIP: %v", readErr)
			}

			if !bytes.Equal(got, original) {
				t.Fatalf(
					"SECURITY FAILURE: altered ZIP published in %q",
					header.Name,
				)
			}

			t.Log(
				"SAFE: original verified ZIP preserved after path replacement",
			)
			return
		}

		t.Logf("replaced inode rejected: %v", err)
	})

	t.Run("in-place change after verified open", func(t *testing.T) {
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

		infoBefore, statErr := f.Stat()
		closeErr := f.Close()
		if statErr != nil || closeErr != nil {
			t.Fatalf(
				"inspect ZIP: %v, %v", statErr, closeErr,
			)
		}

		destination := &triggeredModuleSnapshotWriterV4{
			trigger: func(p []byte) bool {
				return bytes.HasPrefix(
					p,
					[]byte("cache/download/example.com/external/@v/"),
				)
			},
			mutate: func() error {
				file, err := os.OpenFile(
					zipPath, os.O_RDWR, 0,
				)
				if err != nil {
					return err
				}

				one := []byte{0}
				if _, err := file.ReadAt(one, 0); err != nil {
					_ = file.Close()
					return err
				}
				one[0] ^= 0xff

				if _, err := file.WriteAt(one, 0); err != nil {
					_ = file.Close()
					return err
				}
				return file.Close()
			},
		}

		err = WriteGoModuleSnapshotFromStoreV4(
			destination, verified,
		)

		if !destination.called {
			t.Fatalf(
				"dependency header trigger not reached: %v", err,
			)
		}
		if err == nil {
			t.Fatal(
				"SECURITY FAILURE: post-open mutation accepted",
			)
		}
		if destination.output.Len() == 0 {
			t.Fatal(
				"expected incomplete TAR after late mutation",
			)
		}

		infoAfter, statErr := os.Stat(zipPath)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !os.SameFile(infoBefore, infoAfter) ||
			infoBefore.Size() != infoAfter.Size() {
			t.Fatal("mutation unexpectedly changed inode or size")
		}

		t.Logf("post-open mutation rejected: %v", err)
	})
}

func TestWriteGoModuleSnapshotFromStoreV4RejectsPreOpenReplacement(
	t *testing.T,
) {
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
	infoBefore, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil || statErr != nil || closeErr != nil {
		t.Fatalf(
			"read ZIP fixture: %v, %v, %v",
			readErr, statErr, closeErr,
		)
	}
	if len(original) == 0 {
		t.Fatal("empty ZIP fixture")
	}

	replacement := filepath.Join(
		filepath.Dir(zipPath), ".preopen-replacement",
	)

	changed := bytes.Clone(original)
	changed[0] ^= 0xff

	if err := os.WriteFile(replacement, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, zipPath); err != nil {
		t.Fatal(err)
	}

	infoAfter, err := os.Stat(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(infoBefore, infoAfter) {
		t.Fatal("test setup did not replace inode")
	}
	if infoBefore.Size() != infoAfter.Size() {
		t.Fatal("test setup changed artifact size")
	}

	var output bytes.Buffer
	err = WriteGoModuleSnapshotFromStoreV4(
		&output, verified,
	)
	if err == nil {
		t.Fatal("SECURITY FAILURE: replaced inode accepted")
	}
	if output.Len() != 0 {
		t.Fatalf(
			"SECURITY FAILURE: %d bytes emitted before rejection",
			output.Len(),
		)
	}

	t.Logf("pre-open inode replacement rejected: %v", err)
}

func TestWriteGoModuleSnapshotFromStoreV4RejectsLocalIdentityMismatch(
	t *testing.T,
) {
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
		t.Fatal("unexpected local module fixture")
	}

	local := verified.Manifest.LocalModules[0]
	target := local.SnapshotPath + "/go.mod"

	found := false
	var changed []byte

	for i := range verified.Files {
		if verified.Files[i].Path != target {
			continue
		}

		original := string(verified.Files[i].Data)
		old := "module " + local.ModulePath
		newDeclaration := "module example.com/wrong-lib"

		if !strings.Contains(original, old) {
			t.Fatalf("missing original module declaration %q", old)
		}

		changed = []byte(strings.Replace(
			original, old, newDeclaration, 1,
		))
		verified.Files[i].Data = changed
		found = true
		break
	}

	if !found {
		t.Fatalf("missing workspace file %q", target)
	}

	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(changed))

	// Keep file metadata internally consistent so the test
	// exercises module identity rather than digest rejection.
	updatedWorkspace := false
	for i := range verified.Manifest.Workspace.Files {
		f := &verified.Manifest.Workspace.Files[i]
		if f.Path == target {
			f.Size = int64(len(changed))
			f.SHA256 = digest
			updatedWorkspace = true
		}
	}

	if !updatedWorkspace {
		t.Fatal("missing workspace metadata for local go.mod")
	}

	updatedLocal := false
	for i := range verified.Manifest.LocalModules[0].Files {
		f := &verified.Manifest.LocalModules[0].Files[i]
		if f.Path == "go.mod" {
			f.Size = int64(len(changed))
			f.SHA256 = digest
			updatedLocal = true
		}
	}

	if !updatedLocal {
		t.Fatal("missing local module go.mod metadata")
	}

	var output bytes.Buffer
	err = WriteGoModuleSnapshotFromStoreV4(&output, verified)

	if err == nil {
		t.Fatal(
			"SECURITY FAILURE: mismatched local module identity accepted",
		)
	}

	if output.Len() != 0 {
		t.Fatalf(
			"SECURITY FAILURE: %d bytes written before identity rejection",
			output.Len(),
		)
	}

	t.Logf("local module identity rejected: %v", err)
}

func TestWriteGoModuleSnapshotFromStoreV4WorkspaceConsistency(
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
					t.Fatal("fixture requires at least two workspace files")
				}
				v.Files = v.Files[:len(v.Files)-1]
			},
		},
		{
			name: "duplicate workspace path",
			mutate: func(t *testing.T, v *VerifiedStoredCapsuleV4) {
				t.Helper()
				if len(v.Files) < 2 {
					t.Fatal("fixture requires at least two workspace files")
				}
				v.Files[1].Path = v.Files[0].Path
			},
		},
		{
			name: "unexpected workspace path",
			mutate: func(t *testing.T, v *VerifiedStoredCapsuleV4) {
				t.Helper()
				if len(v.Files) == 0 {
					t.Fatal("empty workspace fixture")
				}
				v.Files[0].Path = "app/unexpected-file.go"
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
					t.Errorf("close artifact store: %v", err)
				}
			}()

			tc.mutate(t, &verified)

			var output bytes.Buffer
			err = WriteGoModuleSnapshotFromStoreV4(
				&output, verified,
			)

			if err == nil {
				t.Fatal("SECURITY FAILURE: inconsistent workspace accepted")
			}
			if output.Len() != 0 {
				t.Fatalf(
					"SECURITY FAILURE: wrote %d bytes before rejection",
					output.Len(),
				)
			}
			t.Logf("correctly rejected: %v", err)
		})
	}
}
