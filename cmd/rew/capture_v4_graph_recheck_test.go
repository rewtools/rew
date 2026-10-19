package main

import (
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV4GraphUnchanged(t *testing.T) {
	local := capsule.GoModuleIdentityV4{
		Path:    "example.com/local-lib",
		Version: "v0.0.0",
		Replace: &capsule.GoModuleReplacementV4{
			Path:         "../lib",
			SnapshotPath: "lib",
		},
	}

	external := capsule.GoModuleIdentityV4{
		Path:    "example.com/external",
		Version: "v1.0.0",
	}

	localFromGo := local
	localFromGo.Replace = &capsule.GoModuleReplacementV4{
		Path: "../lib",
	}

	tests := []struct {
		name   string
		before []capsule.GoModuleIdentityV4
		after  []capsule.GoModuleIdentityV4
		want   bool
	}{
		{
			name:   "local only unchanged",
			before: []capsule.GoModuleIdentityV4{local},
			after:  []capsule.GoModuleIdentityV4{localFromGo},
			want:   true,
		},
		{
			name: "mixed unchanged",
			before: []capsule.GoModuleIdentityV4{
				external, local,
			},
			after: []capsule.GoModuleIdentityV4{
				external, localFromGo,
			},
			want: true,
		},
		{
			name: "external version changed",
			before: []capsule.GoModuleIdentityV4{
				external, local,
			},
			after: []capsule.GoModuleIdentityV4{
				{
					Path:    external.Path,
					Version: "v1.1.0",
				},
				localFromGo,
			},
		},
		{
			name: "external disappeared",
			before: []capsule.GoModuleIdentityV4{
				external, local,
			},
			after: []capsule.GoModuleIdentityV4{
				localFromGo,
			},
		},
		{
			name: "external added",
			before: []capsule.GoModuleIdentityV4{
				local,
			},
			after: []capsule.GoModuleIdentityV4{
				external, localFromGo,
			},
		},
		{
			name: "local replacement changed",
			before: []capsule.GoModuleIdentityV4{
				external, local,
			},
			after: []capsule.GoModuleIdentityV4{
				external,
				{
					Path:    local.Path,
					Version: local.Version,
					Replace: &capsule.GoModuleReplacementV4{
						Path: "../other",
					},
				},
			},
		},
		{
			name: "local replacement removed",
			before: []capsule.GoModuleIdentityV4{
				external, local,
			},
			after: []capsule.GoModuleIdentityV4{
				external,
				{
					Path:    local.Path,
					Version: local.Version,
				},
			},
		},
		{
			name:   "empty graph rejected",
			before: []capsule.GoModuleIdentityV4{local},
			after:  nil,
		},
		{
			name: "module order changed",
			before: []capsule.GoModuleIdentityV4{
				external, local,
			},
			after: []capsule.GoModuleIdentityV4{
				localFromGo, external,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := captureV4GraphUnchanged(
				tc.before,
				tc.after,
			)

			if got != tc.want {
				t.Fatalf(
					"graph unchanged=%v, want %v",
					got, tc.want,
				)
			}
		})
	}
}
