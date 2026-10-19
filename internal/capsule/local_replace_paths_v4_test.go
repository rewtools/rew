package capsule

import "testing"

func TestValidateLocalReplacementPathV4(t *testing.T) {
	tests := []struct {
		name         string
		workingDir   string
		replacePath  string
		snapshotPath string
		wantErr      bool
	}{
		{
			name:         "sibling module",
			workingDir:   "app",
			replacePath:  "../lib",
			snapshotPath: "lib",
		},
		{
			name:         "nested local module",
			workingDir:   "app",
			replacePath:  "./vendor-local/lib",
			snapshotPath: "app/vendor-local/lib",
		},
		{
			name:         "parent traversal outside snapshot",
			workingDir:   "app",
			replacePath:  "../../secret",
			snapshotPath: "../secret",
			wantErr:      true,
		},
		{
			name:         "absolute replacement",
			workingDir:   "app",
			replacePath:  "/tmp/lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "mismatched snapshot path",
			workingDir:   "app",
			replacePath:  "../lib",
			snapshotPath: "other",
			wantErr:      true,
		},
		{
			name:         "noncanonical working directory",
			workingDir:   "app/../other",
			replacePath:  "../lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "absolute working directory",
			workingDir:   "/workspace/app",
			replacePath:  "../lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "replacement path with backslash",
			workingDir:   "app",
			replacePath:  `..\lib`,
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "replacement equals main root",
			workingDir:   "app",
			replacePath:  ".",
			snapshotPath: "app",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLocalReplacementPathV4(
				tt.workingDir,
				tt.replacePath,
				tt.snapshotPath,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"ValidateLocalReplacementPathV4(%q, %q, %q) error = %v; wantErr = %v",
					tt.workingDir,
					tt.replacePath,
					tt.snapshotPath,
					err,
					tt.wantErr,
				)
			}
		})
	}
}

func TestValidateLocalReplacementPathV4EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		workingDir   string
		replacePath  string
		snapshotPath string
		wantErr      bool
	}{
		{
			name:         "canonical parent replacement",
			workingDir:   "app",
			replacePath:  "../lib",
			snapshotPath: "lib",
		},
		{
			name:         "nested working directory",
			workingDir:   "project/app",
			replacePath:  "../lib",
			snapshotPath: "project/lib",
		},
		{
			name:         "redundant dot component",
			workingDir:   "app",
			replacePath:  ".././lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "redundant parent components",
			workingDir:   "app",
			replacePath:  "../other/../lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "empty path component",
			workingDir:   "app",
			replacePath:  "..//lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "replacement root itself",
			workingDir:   "app",
			replacePath:  "..",
			snapshotPath: "project",
			wantErr:      true,
		},
		{
			name:         "working directory is root",
			workingDir:   ".",
			replacePath:  "./lib",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "NUL in replacement",
			workingDir:   "app",
			replacePath:  "../lib\x00",
			snapshotPath: "lib",
			wantErr:      true,
		},
		{
			name:         "noncanonical snapshot path",
			workingDir:   "app",
			replacePath:  "../lib",
			snapshotPath: "project/../lib",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLocalReplacementPathV4(
				tt.workingDir,
				tt.replacePath,
				tt.snapshotPath,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"workingDir=%q replacePath=%q snapshotPath=%q: err=%v wantErr=%v",
					tt.workingDir,
					tt.replacePath,
					tt.snapshotPath,
					err,
					tt.wantErr,
				)
			}
		})
	}
}
