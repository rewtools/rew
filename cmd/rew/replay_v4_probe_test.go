package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectReplayV4Kind(t *testing.T) {
	makeArchive := func(t *testing.T, manifest any) string {
		t.Helper()

		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)

		err = tw.WriteHeader(&tar.Header{
			Name: "manifest.json",
			Mode: 0600,
			Size: int64(len(data)),
		})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(t.TempDir(), "test.rew")
		if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}

		return path
	}

	t.Run("local route", func(t *testing.T) {
		path := makeArchive(t, map[string]any{
			"schema_version": 4,
			"go_selected_modules": []any{
				map[string]any{},
			},
		})

		kind, err := inspectReplayV4Kind(path)
		if err != nil || kind != replayV4Local {
			t.Fatalf("kind=%d err=%v", kind, err)
		}
	})

	t.Run("mixed route", func(t *testing.T) {
		path := makeArchive(t, map[string]any{
			"schema_version": 4,
			"go_selected_modules": []any{
				map[string]any{},
				map[string]any{},
			},
			"go_modules": []any{
				map[string]any{},
			},
		})

		kind, err := inspectReplayV4Kind(path)
		if err != nil || kind != replayV4Mixed {
			t.Fatalf("kind=%d err=%v", kind, err)
		}
	})

	t.Run("unsupported layout", func(t *testing.T) {
		path := makeArchive(t, map[string]any{
			"schema_version": 4,
			"go_selected_modules": []any{
				map[string]any{},
				map[string]any{},
				map[string]any{},
			},
		})

		if _, err := inspectReplayV4Kind(path); err == nil {
			t.Fatal("unsupported layout accepted")
		}
	})

	t.Run("wrong schema", func(t *testing.T) {
		path := makeArchive(t, map[string]any{
			"schema_version": 3,
			"go_selected_modules": []any{
				map[string]any{},
			},
		})

		if _, err := inspectReplayV4Kind(path); err == nil {
			t.Fatal("wrong schema accepted")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)

		data := []byte("{invalid")
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

		path := filepath.Join(t.TempDir(), "invalid.rew")
		if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}

		if _, err := inspectReplayV4Kind(path); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	})

	t.Run("symlink rejected", func(t *testing.T) {
		target := makeArchive(t, map[string]any{
			"schema_version": 4,
			"go_selected_modules": []any{
				map[string]any{},
			},
		})

		link := filepath.Join(t.TempDir(), "link.rew")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}

		_, err := inspectReplayV4Kind(link)
		if err == nil {
			t.Fatal("symlink accepted")
		}
	})

	t.Run("missing archive rejected", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.rew")
		if _, err := inspectReplayV4Kind(path); err == nil {
			t.Fatal("missing archive accepted")
		}
	})

	t.Run("arbitrary bytes rejected", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.rew")
		if err := os.WriteFile(
			path,
			[]byte(strings.Repeat("x", 2048)),
			0600,
		); err != nil {
			t.Fatal(err)
		}

		if _, err := inspectReplayV4Kind(path); err == nil {
			t.Fatal("arbitrary bytes accepted")
		}
	})
}

// Routing metadata alone must never authorize Docker execution.
// These archives have a valid routing manifest but deliberately
// lack the complete verified V4 workspace and artifacts.
func TestReplayV4AutomaticDispatchRejectsUnverifiedArchive(
	t *testing.T,
) {
	tests := []struct {
		name       string
		manifest   map[string]any
		diagnostic string
	}{
		{
			name: "local route reaches local verified reader",
			manifest: map[string]any{
				"schema_version": 4,
				"go_selected_modules": []any{
					map[string]any{},
				},
			},
			diagnostic: "verify v4 capsule",
		},
		{
			name: "mixed route reaches mixed verified reader",
			manifest: map[string]any{
				"schema_version": 4,
				"go_selected_modules": []any{
					map[string]any{},
					map[string]any{},
				},
				"go_modules": []any{
					map[string]any{},
				},
			},
			diagnostic: "verify mixed v4 capsule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.manifest)
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

			dir := t.TempDir()
			capsulePath := filepath.Join(dir, "unverified.rew")
			if err := os.WriteFile(
				capsulePath, archive.Bytes(), 0600,
			); err != nil {
				t.Fatal(err)
			}

			stderrPath := filepath.Join(dir, "stderr.txt")
			stderrFile, err := os.OpenFile(
				stderrPath,
				os.O_RDWR|os.O_CREATE|os.O_EXCL,
				0600,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer stderrFile.Close()

			originalStderr := os.Stderr
			code := func() int {
				os.Stderr = stderrFile
				defer func() { os.Stderr = originalStderr }()

				return runReplay([]string{
					"--v4",
					"--allow-command",
					capsulePath,
				})
			}()

			if _, err := stderrFile.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			diagnostic, err := io.ReadAll(stderrFile)
			if err != nil {
				t.Fatal(err)
			}

			if code != 125 {
				t.Fatalf(
					"CLI exit=%d, want 125: %s",
					code, diagnostic,
				)
			}
			if !strings.Contains(
				string(diagnostic), tt.diagnostic,
			) {
				t.Fatalf(
					"route did not reach expected verified reader: %s",
					diagnostic,
				)
			}

			t.Logf(
				"unverified %s rejected before Docker",
				tt.name,
			)
		})
	}
}

// The path-based probe remains compatible, but the descriptor-based
// probe must classify the originally opened capsule after a rename.
func TestInspectReplayV4KindFromFilePinned(t *testing.T) {
	makeArchive := func(
		t *testing.T,
		name string,
		mixed bool,
	) string {
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

		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("nil descriptor rejected", func(t *testing.T) {
		if _, err := inspectReplayV4KindFromFile(nil); err == nil {
			t.Fatal("nil descriptor accepted")
		}
	})

	t.Run("rename does not change classification", func(t *testing.T) {
		originalPath := makeArchive(t, "original.rew", false)
		replacementPath := makeArchive(t, "mixed.rew", true)

		f, err := os.Open(originalPath)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		if err := os.Rename(replacementPath, originalPath); err != nil {
			t.Fatal(err)
		}

		if _, err := f.Seek(512, 0); err != nil {
			t.Fatal(err)
		}

		kind, err := inspectReplayV4KindFromFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if kind != replayV4Local {
			t.Fatalf("pinned kind=%d, want local", kind)
		}

		pathKind, err := inspectReplayV4Kind(originalPath)
		if err != nil {
			t.Fatal(err)
		}
		if pathKind != replayV4Mixed {
			t.Fatalf("replacement path kind=%d, want mixed", pathKind)
		}

		if _, err := f.Stat(); err != nil {
			t.Fatalf("probe closed caller descriptor: %v", err)
		}
	})
}
