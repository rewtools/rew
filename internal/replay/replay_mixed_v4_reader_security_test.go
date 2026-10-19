package replay

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestRunMixedV4ReaderRejectsConsistentHashesButWrongGraph(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "external version disagrees with manifest",
			old:  "example.com/external v1.0.0",
			new:  "example.com/external v1.0.1",
		},
		{
			name: "local replacement disagrees with manifest",
			old:  "replace example.com/local-lib => ../lib",
			new:  "replace example.com/local-lib => ../other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verified := mixedSnapshotFixtureV4(t)

			var original bytes.Buffer
			if err := capsule.WriteWorkspaceFromStoreV4(
				&original,
				verified.Manifest,
				verified.Files,
				verified.Store,
			); err != nil {
				t.Fatalf("build valid control capsule: %v", err)
			}

			var manifest capsule.ManifestV4
			entries := make(map[string][]byte)
			var order []string

			reader := tar.NewReader(bytes.NewReader(original.Bytes()))

			for {
				header, err := reader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}

				data, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				if _, exists := entries[header.Name]; exists {
					t.Fatalf("duplicate fixture entry %q", header.Name)
				}

				entries[header.Name] = data
				order = append(order, header.Name)
			}

			if len(order) == 0 || order[0] != "manifest.json" {
				t.Fatal("unexpected fixture TAR ordering")
			}

			if err := json.Unmarshal(
				entries["manifest.json"],
				&manifest,
			); err != nil {
				t.Fatal(err)
			}

			const workspaceName = "workspace/app/go.mod"
			originalMod, exists := entries[workspaceName]
			if !exists {
				t.Fatal("missing main go.mod in archive")
			}

			if strings.Count(string(originalMod), tc.old) != 1 {
				t.Fatalf("expected one occurrence of %q", tc.old)
			}

			changedMod := []byte(strings.Replace(
				string(originalMod), tc.old, tc.new, 1,
			))
			entries[workspaceName] = changedMod

			updated := false
			for i := range manifest.Workspace.Files {
				if manifest.Workspace.Files[i].Path != "app/go.mod" {
					continue
				}

				manifest.Workspace.Files[i].Size = int64(len(changedMod))
				manifest.Workspace.Files[i].SHA256 = capsule.SHA256(changedMod)
				updated = true
			}
			if !updated {
				t.Fatal("manifest missing app/go.mod metadata")
			}

			newManifest, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			entries["manifest.json"] = newManifest

			var altered bytes.Buffer
			writer := tar.NewWriter(&altered)

			for _, name := range order {
				data := entries[name]
				header := &tar.Header{
					Name: name,
					Mode: 0600,
					Size: int64(len(data)),
				}

				if err := writer.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			filename := filepath.Join(t.TempDir(), "malicious.rew")
			if err := os.WriteFile(filename, altered.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}

			tempRoot := t.TempDir()
			t.Setenv("TMPDIR", tempRoot)
			t.Setenv(
				"DOCKER_HOST",
				"unix:///nonexistent/rew-reader-security/docker.sock",
			)

			_, rejectionErr := RunMixedV4WithOptions(
				context.Background(),
				filename,
				Options{
					AllowArbitraryCommand: true,
					MemoryBytes:           512 << 20,
				},
			)
			if rejectionErr == nil {
				t.Fatal("contradictory mixed V4 graph accepted")
			}

			if !strings.Contains(
				rejectionErr.Error(),
				"invalid mixed v4 dependency graph",
			) {
				t.Fatalf(
					"expected semantic graph rejection, got: %v",
					rejectionErr,
				)
			}
			if strings.Contains(rejectionErr.Error(), "SHA-256 mismatch") {
				t.Fatalf(
					"rejected on file hash instead of graph: %v",
					rejectionErr,
				)
			}
			if strings.Contains(rejectionErr.Error(), "docker SDK") {
				t.Fatalf("reached Docker unexpectedly: %v", rejectionErr)
			}

			remaining, err := os.ReadDir(tempRoot)
			if err != nil {
				t.Fatal(err)
			}
			if len(remaining) != 0 {
				t.Fatalf(
					"reader rejection leaked %d temp entries",
					len(remaining),
				)
			}

			t.Logf("semantic graph rejected: %v", rejectionErr)
		})
	}
}
