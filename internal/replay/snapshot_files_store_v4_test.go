package replay

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
	"golang.org/x/mod/sumdb/dirhash"
)

func TestPrepareSnapshotFilesFromStoreV4FailureCleansUp(
	t *testing.T,
) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	// An invalid verified capsule must never produce
	// usable snapshot readers.
	var verified capsule.VerifiedStoredCapsuleV4

	prepared, err := prepareSnapshotFilesFromStoreV4(verified)
	if err == nil {
		if prepared != nil {
			_ = prepared.Close()
		}
		t.Fatal("invalid mixed V4 capsule accepted")
	}
	if prepared != nil {
		_ = prepared.Close()
		t.Fatal("failed preparation returned snapshot readers")
	}

	remaining, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf(
			"snapshot preparation leaked %d temporary entries",
			len(remaining),
		)
	}
}

// mixedSnapshotFixtureV4 builds a real Go module ZIP and its
// checksum-backed V4 capsule without network access or Docker.
func mixedSnapshotFixtureV4(
	t *testing.T,
) capsule.VerifiedStoredCapsuleV4 {
	t.Helper()

	const modPath = "example.com/external"
	const version = "v1.0.0"

	modData := []byte(
		"module " + modPath + "\n\ngo 1.26.0\n",
	)
	sourceData := []byte(
		"package external\nfunc Value() string { return \"ok\" }\n",
	)

	zipPath := filepath.Join(t.TempDir(), "external.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{modPath + "@" + version + "/go.mod", modData},
		{modPath + "@" + version + "/external.go", sourceData},
	} {
		w, err := zw.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}

	modHash, err := dirhash.Hash1(
		[]string{"go.mod"},
		func(name string) (io.ReadCloser, error) {
			if name != "go.mod" {
				return nil, fmt.Errorf("unexpected mod file %q", name)
			}
			return io.NopCloser(bytes.NewReader(modData)), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	artifacts := map[string][]byte{
		"module-0001.zip":  zipData,
		"module-0001.mod":  modData,
		"module-0001.info": []byte(`{"Version":"v1.0.0"}`),
	}

	var moduleArtifacts []capsule.GoModuleArtifact
	for _, ext := range []string{".zip", ".mod", ".info"} {
		name := "module-0001" + ext
		data := artifacts[name]
		moduleArtifacts = append(
			moduleArtifacts,
			capsule.GoModuleArtifact{
				Path:   name,
				Size:   int64(len(data)),
				SHA256: capsule.SHA256(data),
			},
		)
	}

	external := capsule.GoModuleV3{
		Path:      modPath,
		Version:   version,
		Artifacts: moduleArtifacts,
	}

	goSum := []byte(fmt.Sprintf(
		"%s %s %s\n%s %s/go.mod %s\n",
		modPath, version, zipHash,
		modPath, version, modHash,
	))

	files := []capsule.WorkspaceFile{
		{
			Path: "app/go.mod",
			Data: []byte(
				"module example.com/app\n\n" +
					"go 1.26.0\n\n" +
					"require (\n" +
					"example.com/local-lib v0.0.0\n" +
					modPath + " " + version + "\n" +
					")\n\n" +
					"replace example.com/local-lib => ../lib\n",
			),
			Mode: 0600,
		},
		{Path: "app/go.sum", Data: goSum, Mode: 0600},
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

	placeholder := capsule.FileMetadata{
		Size:   10,
		SHA256: "sha256:" + strings.Repeat("a", 64),
	}

	mainMetadata := placeholder
	mainMetadata.Path = "app/go.mod"
	localWorkspaceMetadata := placeholder
	localWorkspaceMetadata.Path = "lib/go.mod"
	localMetadata := placeholder
	localMetadata.Path = "go.mod"

	manifest := capsule.ManifestV4{
		SchemaVersion: capsule.SchemaVersionV4,
		Execution: capsule.Execution{
			Argv:       []string{"go", "build", "./..."},
			WorkingDir: "app",
			ExitCode:   1,
			Stderr:     "fixture build failure\n",
		},
		Platform: capsule.Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
		Runtime: &capsule.Runtime{
			Kind:    "go",
			Version: capsule.SupportedGoVersion,
		},
		GoBuildEnv: &capsule.GoBuildEnvironment{
			GOOS:       "linux",
			GOARCH:     "amd64",
			CGOEnabled: "0",
		},
		Workspace: capsule.Workspace{
			Files: []capsule.FileMetadata{
				mainMetadata,
				localWorkspaceMetadata,
			},
		},
		GoModules: []capsule.GoModuleV3{external},
		GoSelectedModules: []capsule.GoModuleIdentityV4{
			{
				Path:    modPath,
				Version: version,
			},
			{
				Path:    "example.com/local-lib",
				Version: "v0.0.0",
				Replace: &capsule.GoModuleReplacementV4{
					Path:         "../lib",
					SnapshotPath: "lib",
				},
			},
		},
		LocalModules: []capsule.LocalModuleV4{
			{
				ModulePath:   "example.com/local-lib",
				SnapshotPath: "lib",
				Files:        []capsule.FileMetadata{localMetadata},
			},
		},
	}

	store, err := capsule.NewArtifactStoreV3(
		capsule.MaxGoModuleTotalSizeV3,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close fixture store: %v", err)
		}
	})

	for _, artifact := range moduleArtifacts {
		data := artifacts[artifact.Path]
		if err := store.Put(
			artifact.Path,
			bytes.NewReader(data),
			int64(len(data)),
			strings.TrimPrefix(artifact.SHA256, "sha256:"),
		); err != nil {
			t.Fatal(err)
		}
	}

	var archive bytes.Buffer
	if err := capsule.WriteWorkspaceFromStoreV4(
		&archive, manifest, files, store,
	); err != nil {
		t.Fatalf("write mixed V4 fixture: %v", err)
	}

	filename := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(
		filename, archive.Bytes(), 0600,
	); err != nil {
		t.Fatal(err)
	}

	verified, err := capsule.ReadVerifiedToStoreV4(filename)
	if err != nil {
		t.Fatalf("verify mixed V4 fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := verified.Close(); err != nil {
			t.Errorf("close verified capsule: %v", err)
		}
	})

	return verified
}

func TestPrepareSnapshotFilesFromStoreV4MixedRoundTrip(
	t *testing.T,
) {
	verified := mixedSnapshotFixtureV4(t)

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	prepared, err := prepareSnapshotFilesFromStoreV4(verified)
	if err != nil {
		t.Fatalf("prepare mixed snapshots: %v", err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Errorf("cleanup prepared snapshots: %v", err)
		}
	})

	info, err := os.Stat(prepared.directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("snapshot directory mode = %o", info.Mode().Perm())
	}

	for _, tc := range []struct {
		name string
		file *os.File
	}{
		{"workspace.tar", prepared.workspace},
		{"modules.tar", prepared.modules},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.file == nil {
				t.Fatal("nil prepared TAR")
			}
			info, err := tc.file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() ||
				info.Mode().Perm() != 0600 ||
				info.Size() < 1024 {
				t.Fatalf("invalid prepared TAR: %v", info)
			}

			offset, err := tc.file.Seek(0, io.SeekCurrent)
			if err != nil {
				t.Fatal(err)
			}
			if offset != 0 {
				t.Fatalf("reader offset = %d, want 0", offset)
			}
		})
	}

	wantWorkspace := make(map[string][]byte)
	for _, file := range verified.Files {
		wantWorkspace[file.Path] = file.Data
	}

	tr := tar.NewReader(prepared.workspace)
	workspaceSeen := make(map[string]bool)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		want, ok := wantWorkspace[header.Name]
		if !ok || header.Typeflag != tar.TypeReg ||
			workspaceSeen[header.Name] {
			t.Fatalf("unexpected workspace entry %q", header.Name)
		}
		got, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("workspace mismatch: %q", header.Name)
		}
		workspaceSeen[header.Name] = true
	}
	if len(workspaceSeen) != len(wantWorkspace) {
		t.Fatalf(
			"workspace entries = %d, want %d",
			len(workspaceSeen), len(wantWorkspace),
		)
	}

	mod := verified.Manifest.GoModules[0]

	// Module reader must still be at the beginning.
	if _, err := prepared.modules.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	moduleReader := tar.NewReader(prepared.modules)
	for _, ext := range []string{".zip", ".mod", ".info"} {
		header, err := moduleReader.Next()
		if err != nil {
			t.Fatal(err)
		}

		wantPath := "cache/download/" + mod.Path +
			"/@v/" + mod.Version + ext
		if header.Name != wantPath ||
			header.Typeflag != tar.TypeReg {
			t.Fatalf("unexpected module entry %q", header.Name)
		}

		wantFile := "module-0001" + ext
		stored, err := verified.Store.Open(wantFile)
		if err != nil {
			t.Fatal(err)
		}
		want, readErr := io.ReadAll(stored)
		closeErr := stored.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read stored artifact: %v, %v", readErr, closeErr)
		}
		got, err := io.ReadAll(moduleReader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("module artifact mismatch: %q", wantFile)
		}
	}
	if _, err := moduleReader.Next(); err != io.EOF {
		t.Fatalf("unexpected extra module entry: %v", err)
	}

	directory := prepared.directory
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("snapshot directory remains: %v", err)
	}
	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("leaked temporary entries: %d", len(entries))
	}
}

func TestPrepareSnapshotFilesFromStoreV4SecondWriterFailure(
	t *testing.T,
) {
	verified := mixedSnapshotFixtureV4(t)

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	workspaceCompleted := false
	moduleStarted := false

	prepared, err := prepareSnapshotFilesFromStoreV4WithWriters(
		verified,
		func(w io.Writer) error {
			if err := capsule.WriteWorkspaceSnapshotFromStoreV4(
				w, verified,
			); err != nil {
				return err
			}
			workspaceCompleted = true
			return nil
		},
		func(w io.Writer) error {
			moduleStarted = true
			if _, err := w.Write([]byte("partial TAR")); err != nil {
				return err
			}
			return io.ErrClosedPipe
		},
	)

	if prepared != nil {
		_ = prepared.Close()
		t.Fatal("failed preparation returned snapshot readers")
	}
	if !workspaceCompleted || !moduleStarted {
		t.Fatalf(
			"failure occurred at wrong stage: workspace=%v modules=%v",
			workspaceCompleted, moduleStarted,
		)
	}
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("second writer error not preserved: %v", err)
	}

	remaining, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf(
			"partial snapshot files leaked: %d entries",
			len(remaining),
		)
	}
}

func TestPrepareSnapshotFilesFromStoreV4ReadersBoundToInodes(
	t *testing.T,
) {
	verified := mixedSnapshotFixtureV4(t)

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	prepared, err := prepareSnapshotFilesFromStoreV4(verified)
	if err != nil {
		t.Fatal(err)
	}

	directory := prepared.directory
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Errorf("close snapshots: %v", err)
		}
	}()

	for _, tc := range []struct {
		name string
		file *os.File
	}{
		{"workspace.tar", prepared.workspace},
		{"modules.tar", prepared.modules},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.file == nil {
				t.Fatal("nil snapshot reader")
			}

			originalInfo, err := tc.file.Stat()
			if err != nil {
				t.Fatal(err)
			}

			original, err := io.ReadAll(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if len(original) < 1024 {
				t.Fatal("unexpectedly small original TAR")
			}

			if _, err := tc.file.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(directory, tc.name)
			replacement := path + ".replacement"
			payload := []byte("untrusted replacement, not a TAR")

			if err := os.WriteFile(
				replacement, payload, 0600,
			); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, path); err != nil {
				t.Fatal(err)
			}

			pathInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(originalInfo, pathInfo) {
				t.Fatal("replacement did not change the inode")
			}

			// Confirm the path really points to the replacement.
			onDisk, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(onDisk, payload) {
				t.Fatal("path does not contain replacement bytes")
			}

			// The previously opened descriptor must still return
			// the verified TAR, not the path replacement.
			got, err := io.ReadAll(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Fatal("snapshot reader followed replaced path")
			}
		})
	}

	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("snapshot directory remains: %v", err)
	}

	remaining, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("leaked %d temporary entries", len(remaining))
	}
}
