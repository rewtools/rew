package main

import (
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureExternalModuleIdentitiesV4(t *testing.T) {
	local := capsule.GoModuleIdentityV4{
		Path:    "example.com/local-lib",
		Version: "v0.0.0",
		Replace: &capsule.GoModuleReplacementV4{
			Path: "../lib",
		},
	}

	external := capsule.GoModuleIdentityV4{
		Path:    "example.com/external",
		Version: "v1.0.0",
	}

	tests := []struct {
		name    string
		graph   []capsule.GoModuleIdentityV4
		want    []capsule.GoModuleIdentityV3
		wantErr bool
	}{
		{
			name:  "mixed local first",
			graph: []capsule.GoModuleIdentityV4{local, external},
			want: []capsule.GoModuleIdentityV3{
				{Path: external.Path, Version: external.Version},
			},
		},
		{
			name:  "mixed external first",
			graph: []capsule.GoModuleIdentityV4{external, local},
			want: []capsule.GoModuleIdentityV3{
				{Path: external.Path, Version: external.Version},
			},
		},
		{
			name:  "local only",
			graph: []capsule.GoModuleIdentityV4{local},
		},
		{
			name:    "missing local",
			graph:   []capsule.GoModuleIdentityV4{external},
			wantErr: true,
		},
		{
			name: "too many external modules",
			graph: []capsule.GoModuleIdentityV4{
				local, external,
				{
					Path:    "example.com/another",
					Version: "v1.0.0",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid external version",
			graph: []capsule.GoModuleIdentityV4{
				local,
				{Path: external.Path, Version: "latest"},
			},
			wantErr: true,
		},
		{
			name: "wrong local replacement",
			graph: []capsule.GoModuleIdentityV4{
				{
					Path:    local.Path,
					Version: local.Version,
					Replace: &capsule.GoModuleReplacementV4{
						Path: "../other",
					},
				},
				external,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := captureExternalModuleIdentitiesV4(
				tc.graph, "../lib",
			)

			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid graph accepted")
				}
				if len(got) != 0 {
					t.Fatalf("invalid graph returned modules: %+v", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("valid graph rejected: %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf(
					"got %d external modules, want %d",
					len(got), len(tc.want),
				)
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf(
						"external module %d = %+v, want %+v",
						i, got[i], tc.want[i],
					)
				}
			}
		})
	}
}
