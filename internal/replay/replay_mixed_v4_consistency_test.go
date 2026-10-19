package replay

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestMixedV4WriterRejectsContradictoryGoMod(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "external version differs from selected graph",
			old:  "example.com/external v1.0.0",
			new:  "example.com/external v1.0.1",
		},
		{
			name: "local replacement differs from selected graph",
			old:  "replace example.com/local-lib => ../lib",
			new:  "replace example.com/local-lib => ../other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verified := mixedSnapshotFixtureV4(t)

			files := append(
				[]capsule.WorkspaceFile(nil),
				verified.Files...,
			)

			found := false
			for i := range files {
				if files[i].Path != "app/go.mod" {
					continue
				}
				original := string(files[i].Data)
				if strings.Count(original, tc.old) != 1 {
					t.Fatalf("expected exactly one %q in go.mod", tc.old)
				}
				files[i].Data = []byte(
					strings.Replace(original, tc.old, tc.new, 1),
				)
				found = true
			}
			if !found {
				t.Fatal("fixture missing app/go.mod")
			}

			var output bytes.Buffer
			err := capsule.WriteWorkspaceFromStoreV4(
				&output,
				verified.Manifest,
				files,
				verified.Store,
			)
			if err == nil {
				t.Fatal("contradictory module graph was accepted")
			}
			if !strings.Contains(err.Error(), "v4") {
				t.Fatalf("unexpected rejection: %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf(
					"rejected input wrote %d archive bytes",
					output.Len(),
				)
			}

			t.Logf("inconsistent graph rejected before output: %v", err)
		})
	}
}
