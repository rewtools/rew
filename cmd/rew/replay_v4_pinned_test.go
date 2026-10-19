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

func TestPinnedV4DispatchSurvivesPathReplacement(t *testing.T) {
	writeRoutingArchive := func(
		path string,
		mixed bool,
	) {
		t.Helper()

		manifest := map[string]any{
			"schema_version": 4,
			"go_selected_modules": []any{
				map[string]any{},
			},
		}

		if mixed {
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

		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)

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

		if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}

	root := t.TempDir()
	path := filepath.Join(root, "failure.rew")
	replacement := filepath.Join(root, "replacement.rew")

	writeRoutingArchive(path, false)
	writeRoutingArchive(replacement, true)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Replace the pathname before replay dispatch.
	// Both archives are intentionally incomplete; the engine
	// must fail at full verification, not reach Docker.
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	kind, err := inspectReplayV4KindFromFile(f)
	if err != nil || kind != replayV4Local {
		t.Fatalf("original descriptor kind=%d err=%v", kind, err)
	}

	newKind, err := inspectReplayV4Kind(path)
	if err != nil || newKind != replayV4Mixed {
		t.Fatalf("replacement pathname kind=%d err=%v", newKind, err)
	}

	_, err = runPinnedV4Replay(
		context.Background(),
		f,
		replay.Options{AllowArbitraryCommand: true},
	)
	if err == nil {
		t.Fatal("incomplete pinned capsule was accepted")
	}

	if !strings.Contains(err.Error(), "verify v4 capsule:") {
		t.Fatalf(
			"original local capsule did not reach local reader: %v",
			err,
		)
	}
	if strings.Contains(err.Error(), "verify mixed v4 capsule") ||
		strings.Contains(err.Error(), "Docker") {
		t.Fatalf("unexpected replay stage: %v", err)
	}

	if _, err := f.Stat(); err != nil {
		t.Fatalf("dispatcher closed caller descriptor: %v", err)
	}
}

func TestPinnedV4DispatchRejectsNilDescriptor(t *testing.T) {
	_, err := runPinnedV4Replay(
		context.Background(), nil, replay.Options{},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "routing") {
		t.Fatalf("unexpected nil descriptor result: %v", err)
	}
}
