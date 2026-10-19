package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/replay"
)

func TestV4InPlaceManifestMutationRejectedBeforeDocker(
	t *testing.T,
) {
	tests := []struct {
		name      string
		mixed     bool
		wantKind  replayV4Kind
		wantError string
	}{
		{
			name:      "local",
			wantKind:  replayV4Local,
			wantError: "verify v4 capsule:",
		},
		{
			name:      "mixed",
			mixed:     true,
			wantKind:  replayV4Mixed,
			wantError: "verify mixed v4 capsule:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := map[string]any{
				"schema_version": 4,
				"go_selected_modules": []any{
					map[string]any{},
				},
			}

			if tt.mixed {
				manifest["go_selected_modules"] = []any{
					map[string]any{},
					map[string]any{},
				}
				manifest["go_modules"] = []any{
					map[string]any{},
				}
			}

			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}

			var archive bytes.Buffer
			tw := tar.NewWriter(&archive)

			if err := tw.WriteHeader(&tar.Header{
				Name: "manifest.json",
				Mode: 0600,
				Size: int64(len(data)),
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(data); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(
				t.TempDir(), "mutation.rew",
			)
			if err := os.WriteFile(
				path, archive.Bytes(), 0600,
			); err != nil {
				t.Fatal(err)
			}

			reader, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()

			before, err := reader.Stat()
			if err != nil {
				t.Fatal(err)
			}

			kind, err := inspectReplayV4KindFromFile(
				reader,
			)
			if err != nil || kind != tt.wantKind {
				t.Fatalf(
					"initial kind=%d err=%v",
					kind, err,
				)
			}

			// Modify the same inode after classification.
			// TAR payload begins at offset 512.
			writer, err := os.OpenFile(
				path, os.O_WRONLY, 0,
			)
			if err != nil {
				t.Fatal(err)
			}

			_, writeErr := writer.WriteAt(
				[]byte("!"), 512,
			)
			closeErr := writer.Close()

			if writeErr != nil {
				t.Fatal(writeErr)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}

			after, err := reader.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) {
				t.Fatal("test replaced inode")
			}
			if before.Size() != after.Size() {
				t.Fatal("test changed archive size")
			}

			options := replay.Options{
				AllowArbitraryCommand: true,
			}

			var replayErr error

			switch kind {
			case replayV4Local:
				_, replayErr =
					replay.RunV4WithOptionsFromFile(
						context.Background(),
						reader,
						options,
					)
			case replayV4Mixed:
				_, replayErr =
					replay.RunMixedV4WithOptionsFromFile(
						context.Background(),
						reader,
						options,
					)
			default:
				t.Fatal("unsupported test route")
			}

			if replayErr == nil {
				t.Fatal(
					"in-place corrupted capsule accepted",
				)
			}

			if !strings.Contains(
				replayErr.Error(), tt.wantError,
			) {
				t.Fatalf(
					"wrong rejection stage: %v",
					replayErr,
				)
			}
			if strings.Contains(
				replayErr.Error(), "Docker",
			) {
				t.Fatalf(
					"corruption reached Docker: %v",
					replayErr,
				)
			}

			if _, err := reader.Stat(); err != nil {
				t.Fatalf(
					"caller descriptor closed: %v",
					err,
				)
			}

			t.Logf(
				"%s in-place corruption rejected by verification",
				tt.name,
			)
		})
	}
}
