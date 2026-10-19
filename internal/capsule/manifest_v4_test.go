package capsule

import (
	"encoding/json"
	"testing"
)

func TestManifestV4JSONRoundTrip(t *testing.T) {
	original := ManifestV4{
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
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ManifestV4
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.SchemaVersion != SchemaVersionV4 {
		t.Fatalf("schema version = %d", decoded.SchemaVersion)
	}
	if decoded.Execution.WorkingDir != "app" {
		t.Fatalf("working dir = %q", decoded.Execution.WorkingDir)
	}
	if len(decoded.GoSelectedModules) != 1 ||
		decoded.GoSelectedModules[0].Replace == nil {
		t.Fatal("effective local replacement missing")
	}
	if decoded.GoSelectedModules[0].Replace.Path != "../lib" {
		t.Fatal("relative replacement path changed")
	}
	if len(decoded.LocalModules) != 1 ||
		decoded.LocalModules[0].SnapshotPath != "lib" {
		t.Fatal("local module snapshot metadata missing")
	}
}
