package capsule

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestWriteWorkspaceFromStoreV4(t *testing.T) {
	const externalPath = "example.com/external"

	external, artifacts, goSum := makeIndexedGoModuleFixture(
		t, 1, externalPath,
	)

	makeInput := func() (ManifestV4, []WorkspaceFile) {
		t.Helper()

		manifest := validLocalReplaceManifestV4()

		manifest.Workspace.Files = nil
		manifest.LocalModules[0].Files = nil
		manifest.GoModules = []GoModuleV3{external}

		manifest.GoSelectedModules = []GoModuleIdentityV4{
			{
				Path:    "example.com/local-lib",
				Version: "v0.0.0",
				Replace: &GoModuleReplacementV4{
					Path:         "../lib",
					SnapshotPath: "lib",
				},
			},
			{
				Path:    external.Path,
				Version: external.Version,
			},
		}

		sort.Slice(
			manifest.GoSelectedModules,
			func(i, j int) bool {
				return manifest.GoSelectedModules[i].Path <
					manifest.GoSelectedModules[j].Path
			},
		)

		files := []WorkspaceFile{
			{
				Path: "app/go.mod",
				Data: []byte(
					"module example.com/app\n\n" +
						"go 1.26.0\n\n" +
						"require (\n" +
						" example.com/local-lib v0.0.0\n" +
						" " + external.Path + " " +
						external.Version + "\n" +
						")\n\n" +
						"replace example.com/local-lib => ../lib\n",
				),
				Mode: 0600,
			},
			{
				Path: "app/go.sum",
				Data: append([]byte(nil), goSum...),
				Mode: 0600,
			},
			{
				Path: "app/main.go",
				Data: []byte(
					"package main\n\n" +
						"func main() {}\n",
				),
				Mode: 0600,
			},
			{
				Path: "lib/go.mod",
				Data: []byte(
					"module example.com/local-lib\n\n" +
						"go 1.26.0\n",
				),
				Mode: 0600,
			},
			{
				Path: "lib/lib.go",
				Data: []byte(
					"package local\n\n" +
						"func Hello() {}\n",
				),
				Mode: 0600,
			},
		}

		return manifest, files
	}

	t.Run("valid mixed archive layout", func(t *testing.T) {
		manifest, files := makeInput()

		store := storeModuleFixtureV3(
			t, external, artifacts,
		)

		var output bytes.Buffer

		if err := WriteWorkspaceFromStoreV4(
			&output, manifest, files, store,
		); err != nil {
			t.Fatalf("write mixed archive: %v", err)
		}

		reader := tar.NewReader(
			bytes.NewReader(output.Bytes()),
		)

		expected := []string{
			"manifest.json",
			"workspace/app/go.mod",
			"workspace/app/go.sum",
			"workspace/app/main.go",
			"workspace/lib/go.mod",
			"workspace/lib/lib.go",
		}

		for _, artifact := range external.Artifacts {
			expected = append(
				expected,
				"dependencies/"+artifact.Path,
			)
		}

		for _, name := range expected {
			header, err := reader.Next()
			if err != nil {
				t.Fatalf("read entry %q: %v", name, err)
			}

			if header.Name != name ||
				header.Typeflag != tar.TypeReg {
				t.Fatalf(
					"unexpected entry: got %q, want %q",
					header.Name, name,
				)
			}

			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Fatalf("read entry data %q: %v", name, err)
			}
		}

		if _, err := reader.Next(); err != io.EOF {
			t.Fatalf("unexpected trailing entry: %v", err)
		}
	})

	t.Run("reject wrong go.sum before output", func(t *testing.T) {
		manifest, files := makeInput()

		for index := range files {
			if files[index].Path == "app/go.sum" {
				files[index].Data = []byte("invalid go.sum\n")
			}
		}

		store := storeModuleFixtureV3(
			t, external, artifacts,
		)

		var output bytes.Buffer

		err := WriteWorkspaceFromStoreV4(
			&output, manifest, files, store,
		)

		if err == nil {
			t.Fatal("invalid go.sum accepted")
		}
		if output.Len() != 0 {
			t.Fatalf(
				"writer emitted %d bytes on invalid input",
				output.Len(),
			)
		}
	})

	t.Run("reject missing ZIP before output", func(t *testing.T) {
		manifest, files := makeInput()

		store, err := NewArtifactStoreV3(
			MaxGoModuleTotalSizeV3,
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })

		var output bytes.Buffer

		err = WriteWorkspaceFromStoreV4(
			&output, manifest, files, store,
		)

		if err == nil {
			t.Fatal("missing dependencies accepted")
		}
		if output.Len() != 0 {
			t.Fatalf(
				"writer emitted %d bytes on invalid input",
				output.Len(),
			)
		}
	})

	t.Run("reject missing go.sum before output", func(t *testing.T) {
		manifest, files := makeInput()

		filtered := make([]WorkspaceFile, 0, len(files))
		for _, file := range files {
			if file.Path != "app/go.sum" {
				filtered = append(filtered, file)
			}
		}

		store := storeModuleFixtureV3(t, external, artifacts)

		var output bytes.Buffer
		err := WriteWorkspaceFromStoreV4(
			&output, manifest, filtered, store,
		)

		if err == nil {
			t.Fatal("missing go.sum accepted")
		}
		if output.Len() != 0 {
			t.Fatalf("unexpected output: %d bytes", output.Len())
		}
	})

	t.Run("reject extra stored artifact before output", func(t *testing.T) {
		manifest, files := makeInput()
		store := storeModuleFixtureV3(t, external, artifacts)

		extra := []byte("unexpected dependency")
		if err := store.Put(
			"unexpected.txt",
			bytes.NewReader(extra),
			int64(len(extra)),
			SHA256(extra)[len("sha256:"):],
		); err != nil {
			t.Fatal(err)
		}

		var output bytes.Buffer
		err := WriteWorkspaceFromStoreV4(
			&output, manifest, files, store,
		)

		if err == nil {
			t.Fatal("extra artifact accepted")
		}
		if output.Len() != 0 {
			t.Fatalf("unexpected output: %d bytes", output.Len())
		}
	})

	t.Run("reject mismatched selected graph before output", func(t *testing.T) {
		manifest, files := makeInput()
		store := storeModuleFixtureV3(t, external, artifacts)

		for i := range manifest.GoSelectedModules {
			if manifest.GoSelectedModules[i].Replace == nil {
				manifest.GoSelectedModules[i].Version = "v99.0.0"
			}
		}

		var output bytes.Buffer
		err := WriteWorkspaceFromStoreV4(
			&output, manifest, files, store,
		)

		if err == nil {
			t.Fatal("mismatched selected graph accepted")
		}
		if output.Len() != 0 {
			t.Fatalf("unexpected output: %d bytes", output.Len())
		}
	})

	t.Run("reject nil writer", func(t *testing.T) {
		manifest, files := makeInput()
		store := storeModuleFixtureV3(t, external, artifacts)

		if err := WriteWorkspaceFromStoreV4(
			nil, manifest, files, store,
		); err == nil {
			t.Fatal("nil writer accepted")
		}
	})

	t.Run("partial destination failure", func(t *testing.T) {
		manifest, files := makeInput()
		store := storeModuleFixtureV3(t, external, artifacts)

		destination := &failingStoredWriterV4{
			remaining: 700,
		}

		err := WriteWorkspaceFromStoreV4(
			destination,
			manifest,
			files,
			store,
		)

		if !errors.Is(err, errStoredWriterFailureV4) {
			t.Fatalf(
				"expected destination failure, got %v",
				err,
			)
		}

		if destination.output.Len() != 700 {
			t.Fatalf(
				"partial output = %d bytes, want 700",
				destination.output.Len(),
			)
		}

		if destination.calls < 2 {
			t.Fatalf(
				"failure did not interrupt streaming: calls=%d",
				destination.calls,
			)
		}
	})
}

var errStoredWriterFailureV4 = errors.New(
	"deliberate stored writer destination failure",
)

type failingStoredWriterV4 struct {
	remaining int
	calls     int
	output    bytes.Buffer
}

func (w *failingStoredWriterV4) Write(p []byte) (int, error) {
	w.calls++

	if w.remaining <= 0 {
		return 0, errStoredWriterFailureV4
	}

	n := len(p)
	if n > w.remaining {
		n = w.remaining
	}

	written, err := w.output.Write(p[:n])
	w.remaining -= written

	if err != nil {
		return written, err
	}

	if written < len(p) {
		return written, errStoredWriterFailureV4
	}

	return written, nil
}

// mutationWriterV4 modifies a fixture artifact when archive output
// begins, deterministically placing the modification after preflight.
type mutationWriterV4 struct {
	output bytes.Buffer
	mutate func() error
	called bool
}

func (w *mutationWriterV4) Write(p []byte) (int, error) {
	if !w.called {
		w.called = true
		if err := w.mutate(); err != nil {
			return 0, err
		}
	}
	return w.output.Write(p)
}

func TestWriteWorkspaceFromStoreV4RejectsStoreMutation(t *testing.T) {
	const externalPath = "example.com/external"

	external, artifacts, goSum := makeIndexedGoModuleFixture(
		t, 1, externalPath,
	)

	manifest := validLocalReplaceManifestV4()
	manifest.Workspace.Files = nil
	manifest.LocalModules[0].Files = nil
	manifest.GoModules = []GoModuleV3{external}
	manifest.GoSelectedModules = []GoModuleIdentityV4{
		{
			Path:    external.Path,
			Version: external.Version,
		},
		{
			Path:    "example.com/local-lib",
			Version: "v0.0.0",
			Replace: &GoModuleReplacementV4{
				Path:         "../lib",
				SnapshotPath: "lib",
			},
		},
	}
	sort.Slice(manifest.GoSelectedModules, func(i, j int) bool {
		return manifest.GoSelectedModules[i].Path <
			manifest.GoSelectedModules[j].Path
	})

	files := []WorkspaceFile{
		{
			Path: "app/go.mod",
			Data: []byte(
				"module example.com/app\n\n" +
					"go 1.26.0\n\n" +
					"require (\n" +
					"example.com/local-lib v0.0.0\n" +
					external.Path + " " + external.Version + "\n" +
					")\n\n" +
					"replace example.com/local-lib => ../lib\n",
			),
			Mode: 0600,
		},
		{
			Path: "app/go.sum",
			Data: goSum,
			Mode: 0600,
		},
		{
			Path: "app/main.go",
			Data: []byte("package main\nfunc main() {}\n"),
			Mode: 0600,
		},
		{
			Path: "lib/go.mod",
			Data: []byte("module example.com/local-lib\ngo 1.26.0\n"),
			Mode: 0600,
		},
		{
			Path: "lib/lib.go",
			Data: []byte("package local\nfunc Hello() {}\n"),
			Mode: 0600,
		},
	}

	store := storeModuleFixtureV3(t, external, artifacts)

	// Obtain the private fixture path without changing anything yet.
	zipFile, err := store.Open("module-0001.zip")
	if err != nil {
		t.Fatal(err)
	}
	zipPath := zipFile.Name()
	if err := zipFile.Close(); err != nil {
		t.Fatal(err)
	}

	destination := &mutationWriterV4{
		mutate: func() error {
			// In-place corruption after verification, before
			// the Writer reopens dependency bytes.
			return os.WriteFile(
				zipPath,
				[]byte("corrupted after preflight"),
				0600,
			)
		},
	}

	err = WriteWorkspaceFromStoreV4(
		destination, manifest, files, store,
	)

	if !destination.called {
		t.Fatalf(
			"archive output was never attempted; writer error: %v",
			err,
		)
	}

	if destination.output.Len() == 0 {
		t.Fatal("expected partial archive output")
	}

	if err == nil {
		t.Fatal("post-preflight artifact mutation accepted")
	}

	t.Logf("mutation correctly rejected: %v", err)
}

// dependencyHeaderMutationWriterV4 mutates an artifact synchronously
// when its TAR header is written. At that point store.Open has
// already returned its verified file descriptor.
type dependencyHeaderMutationWriterV4 struct {
	output  bytes.Buffer
	mutate  func() error
	mutated bool
}

func (w *dependencyHeaderMutationWriterV4) Write(p []byte) (int, error) {
	if !w.mutated &&
		bytes.HasPrefix(
			p,
			[]byte("dependencies/module-0001.zip"),
		) {
		w.mutated = true
		if err := w.mutate(); err != nil {
			return 0, err
		}
	}
	return w.output.Write(p)
}

func TestWriteWorkspaceFromStoreV4RejectsOpenFileMutation(t *testing.T) {
	external, artifacts, goSum := makeIndexedGoModuleFixture(
		t, 1, "example.com/external",
	)

	manifest := validLocalReplaceManifestV4()
	manifest.Workspace.Files = nil
	manifest.LocalModules[0].Files = nil
	manifest.GoModules = []GoModuleV3{external}
	manifest.GoSelectedModules = []GoModuleIdentityV4{
		{
			Path:    external.Path,
			Version: external.Version,
		},
		{
			Path:    "example.com/local-lib",
			Version: "v0.0.0",
			Replace: &GoModuleReplacementV4{
				Path:         "../lib",
				SnapshotPath: "lib",
			},
		},
	}

	sort.Slice(manifest.GoSelectedModules, func(i, j int) bool {
		return manifest.GoSelectedModules[i].Path <
			manifest.GoSelectedModules[j].Path
	})

	files := []WorkspaceFile{
		{
			Path: "app/go.mod",
			Data: []byte(
				"module example.com/app\n\n" +
					"go 1.26.0\n\n" +
					"require (\n" +
					"example.com/local-lib v0.0.0\n" +
					external.Path + " " + external.Version + "\n" +
					")\n\n" +
					"replace example.com/local-lib => ../lib\n",
			),
			Mode: 0600,
		},
		{
			Path: "app/go.sum",
			Data: goSum,
			Mode: 0600,
		},
		{
			Path: "app/main.go",
			Data: []byte("package main\nfunc main() {}\n"),
			Mode: 0600,
		},
		{
			Path: "lib/go.mod",
			Data: []byte(
				"module example.com/local-lib\ngo 1.26.0\n",
			),
			Mode: 0600,
		},
		{
			Path: "lib/lib.go",
			Data: []byte("package local\nfunc Hello() {}\n"),
			Mode: 0600,
		},
	}

	store := storeModuleFixtureV3(t, external, artifacts)

	zipFile, err := store.Open("module-0001.zip")
	if err != nil {
		t.Fatal(err)
	}
	zipPath := zipFile.Name()
	if err := zipFile.Close(); err != nil {
		t.Fatal(err)
	}

	destination := &dependencyHeaderMutationWriterV4{
		mutate: func() error {
			// Preserve file size and inode to exercise the
			// integrity check AFTER store.Open succeeds.
			file, err := os.OpenFile(
				zipPath, os.O_WRONLY, 0,
			)
			if err != nil {
				return err
			}

			_, writeErr := file.WriteAt([]byte{'X'}, 0)
			closeErr := file.Close()

			if writeErr != nil {
				return writeErr
			}
			return closeErr
		},
	}

	err = WriteWorkspaceFromStoreV4(
		destination, manifest, files, store,
	)

	if !destination.mutated {
		t.Fatalf(
			"dependency TAR header was never reached: %v",
			err,
		)
	}

	if err == nil {
		t.Fatal("modified open artifact was accepted")
	}

	if !strings.Contains(
		err.Error(),
		"changed artifact while writing",
	) {
		t.Fatalf(
			"expected streaming digest rejection, got: %v",
			err,
		)
	}

	if destination.output.Len() == 0 {
		t.Fatal("expected incomplete archive output")
	}

	t.Logf("open-file mutation rejected: %v", err)
}
