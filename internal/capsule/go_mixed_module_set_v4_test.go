package capsule

import "testing"

func TestValidateMixedGoModuleSetV4(t *testing.T) {
	local := GoModuleIdentityV4{
		Path:    "example.com/local-lib",
		Version: "v0.0.0",
		Replace: &GoModuleReplacementV4{
			Path:         "../lib",
			SnapshotPath: "lib",
		},
	}

	external := GoModuleIdentityV4{
		Path:    "example.com/external",
		Version: "v1.2.3",
	}

	artifact := GoModuleV3{
		Path:    external.Path,
		Version: external.Version,
	}

	tests := []struct {
		name     string
		selected []GoModuleIdentityV4
		full     []GoModuleV3
		wantErr  bool
	}{
		{
			name:     "valid mixed graph",
			selected: []GoModuleIdentityV4{external, local},
			full:     []GoModuleV3{artifact},
		},
		{
			name:     "missing external artifact",
			selected: []GoModuleIdentityV4{external, local},
			wantErr:  true,
		},
		{
			name:     "unexpected external artifact",
			selected: []GoModuleIdentityV4{local},
			full:     []GoModuleV3{artifact},
			wantErr:  true,
		},
		{
			name:     "wrong external version",
			selected: []GoModuleIdentityV4{external, local},
			full: []GoModuleV3{{
				Path:    external.Path,
				Version: "v1.2.4",
			}},
			wantErr: true,
		},
		{
			name:     "duplicate external artifact",
			selected: []GoModuleIdentityV4{external, local},
			full:     []GoModuleV3{artifact, artifact},
			wantErr:  true,
		},
		{
			name:     "missing local replacement",
			selected: []GoModuleIdentityV4{external},
			full:     []GoModuleV3{artifact},
			wantErr:  true,
		},
		{
			name:     "duplicate selected external module",
			selected: []GoModuleIdentityV4{external, external, local},
			full:     []GoModuleV3{artifact},
			wantErr:  true,
		},
		{
			name: "extra selected external module",
			selected: []GoModuleIdentityV4{
				external,
				{
					Path:    "example.com/extra",
					Version: "v1.0.0",
				},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "wrong local module version",
			selected: []GoModuleIdentityV4{
				external,
				{
					Path:    "example.com/local-lib",
					Version: "",
					Replace: local.Replace,
				},
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "invalid external module path",
			selected: []GoModuleIdentityV4{
				{Path: "../external", Version: "v1.2.3"},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "invalid external module version",
			selected: []GoModuleIdentityV4{
				{Path: external.Path, Version: "not-a-version"},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name:     "unsorted selected modules",
			selected: []GoModuleIdentityV4{local, external},
			full:     []GoModuleV3{artifact},
			wantErr:  true,
		},
		{
			name: "invalid local snapshot path",
			selected: []GoModuleIdentityV4{
				external,
				{
					Path:    local.Path,
					Version: local.Version,
					Replace: &GoModuleReplacementV4{
						Path:         "../lib",
						SnapshotPath: "../lib",
					},
				},
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},

		{
			name: "empty local module path",
			selected: []GoModuleIdentityV4{
				external,
				{
					Path:    "",
					Version: "v0.0.0",
					Replace: local.Replace,
				},
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "invalid local module path",
			selected: []GoModuleIdentityV4{
				external,
				{
					Path:    "../local-lib",
					Version: "v0.0.0",
					Replace: local.Replace,
				},
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "external module masquerades as local",
			selected: []GoModuleIdentityV4{
				{
					Path:    external.Path,
					Version: external.Version,
					Replace: local.Replace,
				},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "selected external path changed",
			selected: []GoModuleIdentityV4{
				{
					Path:    "example.com/different",
					Version: external.Version,
				},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "empty external module version",
			selected: []GoModuleIdentityV4{
				{
					Path:    external.Path,
					Version: "",
				},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
		{
			name: "two local replacements",
			selected: []GoModuleIdentityV4{
				{
					Path:    external.Path,
					Version: external.Version,
					Replace: local.Replace,
				},
				local,
			},
			full:    []GoModuleV3{artifact},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMixedGoModuleSetV4(
				tt.selected,
				tt.full,
			)

			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
