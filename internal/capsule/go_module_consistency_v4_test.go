package capsule

import "testing"

func TestValidateGoModuleConsistencyV4(t *testing.T) {
	const validGoMod = `module example.com/main-app

go 1.25

require example.com/local-lib v0.0.0

replace example.com/local-lib => ../lib
`

	validGraph := func() []GoModuleIdentityV4 {
		return []GoModuleIdentityV4{
			{
				Path:    "example.com/local-lib",
				Version: "v0.0.0",
				Replace: &GoModuleReplacementV4{
					Path:         "../lib",
					SnapshotPath: "lib",
				},
			},
		}
	}

	tests := []struct {
		name    string
		goMod   string
		mutate  func([]GoModuleIdentityV4)
		wantErr bool
	}{
		{
			name:  "matching local replacement",
			goMod: validGoMod,
		},
		{
			name:  "replacement path mismatch",
			goMod: validGoMod,
			mutate: func(graph []GoModuleIdentityV4) {
				graph[0].Replace.Path = "../other"
			},
			wantErr: true,
		},
		{
			name: "missing replacement in go.mod",
			goMod: `module example.com/main-app
go 1.25
require example.com/local-lib v0.0.0
`,
			wantErr: true,
		},
		{
			name: "wrong required version",
			goMod: `module example.com/main-app
go 1.25
require example.com/local-lib v0.0.1
replace example.com/local-lib => ../lib
`,
			wantErr: true,
		},
		{
			name: "version-specific replacement",
			goMod: `module example.com/main-app
go 1.25
require example.com/local-lib v0.0.0
replace example.com/local-lib v0.0.0 => ../lib
`,
			wantErr: true,
		},
		{
			name: "exclude directive",
			goMod: `module example.com/main-app
go 1.25
require example.com/local-lib v0.0.0
replace example.com/local-lib => ../lib
exclude example.com/other v1.0.0
`,
			wantErr: true,
		},
		{
			name:  "graph replacement missing",
			goMod: validGoMod,
			mutate: func(graph []GoModuleIdentityV4) {
				graph[0].Replace = nil
			},
			wantErr: true,
		},
		{
			name:    "invalid go.mod",
			goMod:   "not a valid go.mod!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := validGraph()
			if tt.mutate != nil {
				tt.mutate(graph)
			}

			err := ValidateGoModuleConsistencyV4(
				[]byte(tt.goMod),
				graph,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"ValidateGoModuleConsistencyV4() error = %v, wantErr=%v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}
