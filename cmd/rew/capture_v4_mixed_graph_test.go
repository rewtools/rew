package main

import (
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestClassifyCaptureModuleGraphV4(t *testing.T) {
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
		name         string
		modules      []capsule.GoModuleIdentityV4
		replacePath  string
		wantLocal    int
		wantExternal int
		wantError    bool
	}{
		{
			name:         "legacy local only",
			modules:      []capsule.GoModuleIdentityV4{local},
			replacePath:  "../lib",
			wantLocal:    0,
			wantExternal: -1,
		},
		{
			name: "mixed local first",
			modules: []capsule.GoModuleIdentityV4{
				local, external,
			},
			replacePath:  "../lib",
			wantLocal:    0,
			wantExternal: 1,
		},
		{
			name: "mixed external first",
			modules: []capsule.GoModuleIdentityV4{
				external, local,
			},
			replacePath:  "../lib",
			wantLocal:    1,
			wantExternal: 0,
		},
		{
			name: "wrong replacement path",
			modules: []capsule.GoModuleIdentityV4{
				local, external,
			},
			replacePath: "../other",
			wantError:   true,
		},
		{
			name: "two external modules",
			modules: []capsule.GoModuleIdentityV4{
				local, external,
				{
					Path:    "example.com/another",
					Version: "v1.0.0",
				},
			},
			replacePath: "../lib",
			wantError:   true,
		},
		{
			name:        "missing local module",
			modules:     []capsule.GoModuleIdentityV4{external},
			replacePath: "../lib",
			wantError:   true,
		},
		{
			name:      "empty graph",
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			localIndex, externalIndex, err :=
				classifyCaptureModuleGraphV4(
					tc.modules,
					tc.replacePath,
				)

			if tc.wantError {
				if err == nil {
					t.Fatal("unsupported graph accepted")
				}
				return
			}

			if err != nil {
				t.Fatalf("valid graph rejected: %v", err)
			}

			if localIndex != tc.wantLocal ||
				externalIndex != tc.wantExternal {
				t.Fatalf(
					"indices local=%d external=%d, want %d/%d",
					localIndex,
					externalIndex,
					tc.wantLocal,
					tc.wantExternal,
				)
			}
		})
	}
}

func TestClassifyCaptureModuleGraphV4Security(t *testing.T) {
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
		modules []capsule.GoModuleIdentityV4
	}{
		{
			name: "duplicate local path",
			modules: []capsule.GoModuleIdentityV4{
				local, local,
			},
		},
		{
			name: "duplicate external path",
			modules: []capsule.GoModuleIdentityV4{
				local,
				{
					Path:    local.Path,
					Version: "v1.0.0",
				},
			},
		},
		{
			name: "invalid local version",
			modules: []capsule.GoModuleIdentityV4{
				{
					Path:    local.Path,
					Version: "v1.2.3",
					Replace: &capsule.GoModuleReplacementV4{
						Path: "../lib",
					},
				},
				external,
			},
		},
		{
			name: "wrong local snapshot path",
			modules: []capsule.GoModuleIdentityV4{
				{
					Path:    local.Path,
					Version: local.Version,
					Replace: &capsule.GoModuleReplacementV4{
						Path:         "../lib",
						SnapshotPath: "other",
					},
				},
				external,
			},
		},
		{
			name: "external version missing",
			modules: []capsule.GoModuleIdentityV4{
				local,
				{Path: external.Path},
			},
		},
		{
			name: "external version is latest",
			modules: []capsule.GoModuleIdentityV4{
				local,
				{
					Path:    external.Path,
					Version: "latest",
				},
			},
		},
		{
			name: "external path traversal",
			modules: []capsule.GoModuleIdentityV4{
				local,
				{
					Path:    "../external",
					Version: "v1.0.0",
				},
			},
		},
		{
			name: "local path traversal",
			modules: []capsule.GoModuleIdentityV4{
				{
					Path:    "../local",
					Version: "v0.0.0",
					Replace: &capsule.GoModuleReplacementV4{
						Path: "../lib",
					},
				},
				external,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			localIndex, externalIndex, err :=
				classifyCaptureModuleGraphV4(
					tc.modules, "../lib",
				)

			if err == nil {
				t.Fatalf(
					"unsafe graph accepted: local=%d external=%d",
					localIndex, externalIndex,
				)
			}

			if localIndex != -1 || externalIndex != -1 {
				t.Fatalf(
					"rejected graph returned usable indices: %d/%d",
					localIndex, externalIndex,
				)
			}

			t.Logf("correctly rejected: %v", err)
		})
	}
}
