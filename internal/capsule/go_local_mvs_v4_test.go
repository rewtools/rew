package capsule

import "testing"

func TestValidateGoModuleConsistencyV4LocalReplacementMVS(t *testing.T) {
	const goMod = `module example.com/main-app

go 1.25

require example.com/local-lib v0.0.0

replace example.com/local-lib => ../lib
`

	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "same version",
			version: "v0.0.0",
		},
		{
			name:    "higher selected version",
			version: "v0.0.1",
		},
		{
			name:    "lower selected version",
			version: "v0.0.0-0.20200101000000-000000000000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := []GoModuleIdentityV4{
				{
					Path:    "example.com/local-lib",
					Version: tt.version,
					Replace: &GoModuleReplacementV4{
						Path:         "../lib",
						SnapshotPath: "lib",
					},
				},
			}

			err := ValidateGoModuleConsistencyV4(
				[]byte(goMod),
				selected,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"version=%q: error=%v, wantErr=%v",
					tt.version,
					err,
					tt.wantErr,
				)
			}
		})
	}
}
