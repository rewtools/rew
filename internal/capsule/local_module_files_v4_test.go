package capsule

import (
	"strings"
	"testing"
)

func validFileMetadataV4(name string) FileMetadata {
	return FileMetadata{
		Path:   name,
		Size:   10,
		SHA256: "sha256:" + strings.Repeat("a", 64),
	}
}

func validManifestWithFilesV4() ManifestV4 {
	m := validLocalReplaceManifestV4()

	m.Workspace.Files = []FileMetadata{
		validFileMetadataV4("app/go.mod"),
		validFileMetadataV4("app/main.go"),
		validFileMetadataV4("lib/go.mod"),
		validFileMetadataV4("lib/lib.go"),
	}

	m.LocalModules[0].Files = []FileMetadata{
		validFileMetadataV4("go.mod"),
		validFileMetadataV4("lib.go"),
	}

	return m
}

func TestManifestV4LocalModuleFiles(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ManifestV4)
		wantErr bool
	}{
		{name: "valid local module files"},
		{
			name: "missing main go.mod",
			mutate: func(m *ManifestV4) {
				m.Workspace.Files = m.Workspace.Files[1:]
			},
			wantErr: true,
		},
		{
			name: "missing local go.mod",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files =
					m.LocalModules[0].Files[1:]
			},
			wantErr: true,
		},
		{
			name: "local file missing from workspace",
			mutate: func(m *ManifestV4) {
				m.Workspace.Files =
					m.Workspace.Files[:3]
			},
			wantErr: true,
		},
		{
			name: "local file digest mismatch",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files[1].SHA256 =
					"sha256:" + strings.Repeat("b", 64)
			},
			wantErr: true,
		},
		{
			name: "local file size mismatch",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files[1].Size = 99
			},
			wantErr: true,
		},
		{
			name: "duplicate local file",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files = append(
					m.LocalModules[0].Files,
					m.LocalModules[0].Files[0],
				)
			},
			wantErr: true,
		},
		{
			name: "local file escapes directory",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files[1].Path = "../app/main.go"
			},
			wantErr: true,
		},
		{
			name: "absolute local file path",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files[1].Path = "/tmp/lib.go"
			},
			wantErr: true,
		},
		{
			name: "corrupt local digest",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].Files[1].SHA256 = "sha256:bad"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifestWithFilesV4()
			if tt.mutate != nil {
				tt.mutate(&m)
			}

			err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestManifestV4RejectsEmptyFileMetadata(t *testing.T) {
	m := validLocalReplaceManifestV4()

	m.Workspace.Files = nil
	m.LocalModules[0].Files = nil

	if err := m.Validate(); err == nil {
		t.Fatal("empty workspace and local module files must be rejected")
	}
}

func TestManifestV4RejectsUnlistedLocalWorkspaceFiles(t *testing.T) {
	m := validManifestWithFilesV4()

	m.Workspace.Files = append(
		m.Workspace.Files,
		validFileMetadataV4("lib/extra.go"),
	)

	if err := m.Validate(); err == nil {
		t.Fatal("unlisted local workspace file must be rejected")
	}
}

func TestManifestV4RejectsLocalModuleRootAsFile(t *testing.T) {
	m := validManifestWithFilesV4()

	m.Workspace.Files = append(
		m.Workspace.Files,
		validFileMetadataV4("lib"),
	)

	if err := m.Validate(); err == nil {
		t.Fatal("local module root cannot also be a file")
	}
}
