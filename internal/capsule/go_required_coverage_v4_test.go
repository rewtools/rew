package capsule

import "testing"

func TestValidateGoModuleConsistencyV4RequiredCoverage(t *testing.T) {
	const goMod = `module example.com/main-app

go 1.25

require (
	example.com/local-lib v0.0.0
	example.com/external v1.2.0
)

replace example.com/local-lib => ../lib
`

	graph := func() []GoModuleIdentityV4 {
		return []GoModuleIdentityV4{
			{
				Path:    "example.com/local-lib",
				Version: "v0.0.0",
				Replace: &GoModuleReplacementV4{
					Path:         "../lib",
					SnapshotPath: "lib",
				},
			},
			{
				Path:    "example.com/external",
				Version: "v1.2.0",
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func([]GoModuleIdentityV4) []GoModuleIdentityV4
		wantErr bool
	}{
		{
			name: "all required modules present",
		},
		{
			name: "missing external required module",
			mutate: func(items []GoModuleIdentityV4) []GoModuleIdentityV4 {
				return items[:1]
			},
			wantErr: true,
		},
		{
			name: "selected external version below requirement",
			mutate: func(items []GoModuleIdentityV4) []GoModuleIdentityV4 {
				items[1].Version = "v1.1.0"
				return items
			},
			wantErr: true,
		},
		{
			name: "selected external version above requirement",
			mutate: func(items []GoModuleIdentityV4) []GoModuleIdentityV4 {
				items[1].Version = "v1.3.0"
				return items
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := graph()
			if tt.mutate != nil {
				items = tt.mutate(items)
			}

			err := ValidateGoModuleConsistencyV4([]byte(goMod), items)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
