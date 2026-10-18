package capsule

import (
	"bytes"
	"testing"
)

func TestWriteWorkspaceV4RejectsInvalidInputBeforeOutput(t *testing.T) {
	baseFiles := func() []WorkspaceFile {
		return []WorkspaceFile{
			{
				Path: "app/go.mod",
				Data: []byte("module example.com/app\n"),
				Mode: 0600,
			},
			{
				Path: "app/main.go",
				Data: []byte("package main\nfunc main() {}\n"),
				Mode: 0600,
			},
			{
				Path: "lib/go.mod",
				Data: []byte("module example.com/local-lib\n"),
				Mode: 0600,
			},
			{
				Path: "lib/lib.go",
				Data: []byte("package local\n"),
				Mode: 0600,
			},
		}
	}

	tests := []struct {
		name   string
		mutate func(*ManifestV4, *[]WorkspaceFile)
	}{
		{
			name: "duplicate workspace path",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				*files = append(*files, (*files)[0])
			},
		},
		{
			name: "path traversal",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				(*files)[1].Path = "../secret.go"
			},
		},
		{
			name: "absolute path",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				(*files)[1].Path = "/etc/passwd"
			},
		},
		{
			name: "file outside module roots",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				(*files)[1].Path = "other/main.go"
			},
		},
		{
			name: "missing main go.mod",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				*files = (*files)[1:]
			},
		},
		{
			name: "missing local go.mod",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				*files = append((*files)[:2], (*files)[3:]...)
			},
		},
		{
			name: "overlapping module roots",
			mutate: func(m *ManifestV4, _ *[]WorkspaceFile) {
				m.LocalModules[0].SnapshotPath = "app/lib"
			},
		},
		{
			name: "wrong schema",
			mutate: func(m *ManifestV4, _ *[]WorkspaceFile) {
				m.SchemaVersion = SchemaVersionV3
			},
		},
		{
			name: "successful execution",
			mutate: func(m *ManifestV4, _ *[]WorkspaceFile) {
				m.Execution.ExitCode = 0
			},
		},
		{
			name: "unsupported dependency artifact",
			mutate: func(m *ManifestV4, _ *[]WorkspaceFile) {
				m.GoModules = []GoModuleV3{{}}
			},
		},
		{
			name: "file directory conflict",
			mutate: func(_ *ManifestV4, files *[]WorkspaceFile) {
				*files = append(*files, WorkspaceFile{
					Path: "app",
					Data: []byte("conflict"),
					Mode: 0600,
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := validLocalReplaceManifestV4()
			files := baseFiles()

			tc.mutate(&manifest, &files)

			var output bytes.Buffer
			err := WriteWorkspaceV4(&output, manifest, files)

			if err == nil {
				t.Fatal("expected writer rejection")
			}
			if output.Len() != 0 {
				t.Fatalf(
					"writer emitted %d bytes on invalid input",
					output.Len(),
				)
			}
		})
	}
}

func TestWriteWorkspaceV4RejectsNilWriter(t *testing.T) {
	manifest := validLocalReplaceManifestV4()
	err := WriteWorkspaceV4(nil, manifest, nil)
	if err == nil {
		t.Fatal("expected nil writer rejection")
	}
}
