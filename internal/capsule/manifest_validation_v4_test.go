package capsule

import (
	"strings"
	"testing"
)

func validLocalReplaceManifestV4() ManifestV4 {
	return ManifestV4{
		SchemaVersion: SchemaVersionV4,
		Execution: Execution{
			Argv:       []string{"go", "build", "./..."},
			WorkingDir: "app",
			ExitCode:   1,
		},
		Platform: Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
		Runtime: &Runtime{
			Kind:    "go",
			Version: SupportedGoVersion,
		},
		GoBuildEnv: &GoBuildEnvironment{
			GOOS:       "linux",
			GOARCH:     "amd64",
			CGOEnabled: "0",
		},
		Workspace: Workspace{
			Files: []FileMetadata{
				{
					Path:   "app/go.mod",
					Size:   10,
					SHA256: "sha256:" + strings.Repeat("a", 64),
				},
				{
					Path:   "lib/go.mod",
					Size:   10,
					SHA256: "sha256:" + strings.Repeat("a", 64),
				},
			},
		},
		GoSelectedModules: []GoModuleIdentityV4{
			{
				Path:    "example.com/local-lib",
				Version: "v0.0.0",
				Replace: &GoModuleReplacementV4{
					Path:         "../lib",
					SnapshotPath: "lib",
				},
			},
		},
		LocalModules: []LocalModuleV4{
			{
				ModulePath:   "example.com/local-lib",
				SnapshotPath: "lib",
				Files: []FileMetadata{
					{
						Path:   "go.mod",
						Size:   10,
						SHA256: "sha256:" + strings.Repeat("a", 64),
					},
				},
			},
		},
	}
}

func TestManifestV4Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ManifestV4)
		wantErr bool
	}{
		{
			name: "valid local replacement",
		},
		{
			name: "wrong schema version",
			mutate: func(m *ManifestV4) {
				m.SchemaVersion = SchemaVersionV3
			},
			wantErr: true,
		},
		{
			name: "missing local module",
			mutate: func(m *ManifestV4) {
				m.LocalModules = nil
			},
			wantErr: true,
		},
		{
			name: "mismatched replacement path",
			mutate: func(m *ManifestV4) {
				m.GoSelectedModules[0].Replace.SnapshotPath = "other"
			},
			wantErr: true,
		},
		{
			name: "duplicate selected module",
			mutate: func(m *ManifestV4) {
				m.GoSelectedModules = append(
					m.GoSelectedModules,
					m.GoSelectedModules[0],
				)
			},
			wantErr: true,
		},
		{
			name: "duplicate local module",
			mutate: func(m *ManifestV4) {
				m.LocalModules = append(
					m.LocalModules,
					m.LocalModules[0],
				)
			},
			wantErr: true,
		},
		{
			name: "local module missing from graph",
			mutate: func(m *ManifestV4) {
				m.GoSelectedModules = nil
			},
			wantErr: true,
		},
		{
			name: "module identity mismatch",
			mutate: func(m *ManifestV4) {
				m.LocalModules[0].ModulePath = "example.com/different"
			},
			wantErr: true,
		},
		{
			name: "replacement escapes snapshot",
			mutate: func(m *ManifestV4) {
				m.GoSelectedModules[0].Replace.Path = "../../outside"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validLocalReplaceManifestV4()

			if tt.mutate != nil {
				tt.mutate(&manifest)
			}

			err := manifest.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"Validate() error = %v; wantErr = %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}
