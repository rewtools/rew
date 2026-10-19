package capsule

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func mixedStoredArchiveFixtureV4(t *testing.T) string {
	t.Helper()

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
			Data: append([]byte(nil), goSum...),
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

	var archive bytes.Buffer
	if err := WriteWorkspaceFromStoreV4(
		&archive, manifest, files, store,
	); err != nil {
		t.Fatalf("create mixed V4 fixture: %v", err)
	}

	filename := filepath.Join(t.TempDir(), "failure.rew")
	if err := os.WriteFile(
		filename, archive.Bytes(), 0600,
	); err != nil {
		t.Fatal(err)
	}

	return filename
}

func TestReadVerifiedToStoreV4MixedRoundTrip(t *testing.T) {
	filename := mixedStoredArchiveFixtureV4(t)

	verified, err := ReadVerifiedToStoreV4(filename)
	if err != nil {
		t.Fatalf("read verified mixed V4 archive: %v", err)
	}
	defer func() {
		if err := verified.Close(); err != nil {
			t.Errorf("close verified artifact store: %v", err)
		}
	}()

	if verified.Manifest.SchemaVersion != SchemaVersionV4 {
		t.Fatalf(
			"schema = %d; want %d",
			verified.Manifest.SchemaVersion, SchemaVersionV4,
		)
	}

	if len(verified.Manifest.GoModules) != 1 {
		t.Fatalf(
			"external modules = %d; want 1",
			len(verified.Manifest.GoModules),
		)
	}

	if len(verified.Manifest.GoSelectedModules) != 2 {
		t.Fatalf(
			"selected modules = %d; want 2",
			len(verified.Manifest.GoSelectedModules),
		)
	}

	if len(verified.Files) != 5 {
		t.Fatalf(
			"workspace files = %d; want 5",
			len(verified.Files),
		)
	}

	if verified.Store == nil {
		t.Fatal("verified module store missing")
	}

	for _, artifact := range verified.Manifest.GoModules[0].Artifacts {
		file, err := verified.Store.Open(artifact.Path)
		if err != nil {
			t.Fatalf(
				"verified artifact %s unavailable: %v",
				artifact.Path, err,
			)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadVerifiedToStoreV4CloseLifecycle(t *testing.T) {
	filename := mixedStoredArchiveFixtureV4(t)

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	verified, err := ReadVerifiedToStoreV4(filename)
	if err != nil {
		t.Fatalf("read verified capsule: %v", err)
	}

	if verified.Store == nil {
		t.Fatal("missing verified artifact store")
	}

	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		_ = verified.Close()
		t.Fatalf(
			"expected one private artifact directory, got %d",
			len(entries),
		)
	}

	if !entries[0].IsDir() {
		_ = verified.Close()
		t.Fatal("artifact store is not a directory")
	}

	if err := verified.Close(); err != nil {
		t.Fatalf("close verified capsule: %v", err)
	}

	if verified.Store != nil {
		t.Fatal("Store remains accessible after Close")
	}

	remaining, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(remaining) != 0 {
		t.Fatalf(
			"artifact directory remains after Close: %d entries",
			len(remaining),
		)
	}

	if err := verified.Close(); err != nil {
		t.Fatalf("repeated Close failed: %v", err)
	}
}
