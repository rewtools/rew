package docker

import "testing"

func TestDockerExecWorkingDirectoryV4(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{"V3 root", "/workspace", false},
		{"V4 main module", "/workspace/app", false},
		{"V4 sibling module", "/workspace/lib", true},
		{"host root", "/", true},
		{"relative directory", "app", true},
		{"parent traversal", "/workspace/app/../lib", true},
		{"outside workspace", "/tmp", true},
		{"empty directory", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSnapshotExecWorkingDirV4(tc.dir)

			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"directory %q: error=%v, wantErr=%v",
					tc.dir, err, tc.wantErr,
				)
			}
		})
	}
}
