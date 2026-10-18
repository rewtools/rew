package main

import (
	"fmt"
	"github.com/rewtools/rew/internal/capsule"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV3GoEmbedCombinedFileLimit(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	for _, tc := range []struct {
		name          string
		embeddedCount int
		wantExit      int
		wantCapsule   bool
		realGoSum     bool
	}{
		{"2047 selected plus virtual go.sum", 2045, 1, true, false},
		{"2048 selected without go.sum rejected", 2046, 125, false, false},
		{"2048 selected with real go.sum", 2045, 1, true, true},
		{"2049 selected files rejected", 2047, 125, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			assets := filepath.Join(root, "assets")

			if err := os.MkdirAll(assets, 0700); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(
				filepath.Join(root, "go.mod"),
				[]byte("module example.com/embed-limit\n\ngo 1.26\n"),
				0600,
			); err != nil {
				t.Fatal(err)
			}

			source := `package main

import "embed"

//go:embed assets/*
var resources embed.FS

func main() {
    _ = resources
    rewIntentionallyUndefined()
}
`
			if err := os.WriteFile(
				filepath.Join(root, "main.go"), []byte(source), 0600,
			); err != nil {
				t.Fatal(err)
			}

			for i := 0; i < tc.embeddedCount; i++ {
				name := filepath.Join(assets, fmt.Sprintf("file-%04d.txt", i))
				if err := os.WriteFile(name, []byte("x\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			includes := []string{"go.mod", "main.go"}
			if tc.realGoSum {
				if err := os.WriteFile(
					filepath.Join(root, "go.sum"), []byte{}, 0600,
				); err != nil {
					t.Fatal(err)
				}
				includes = append(includes, "go.sum")
			}

			output := filepath.Join(root, "failure.bfc")

			code := captureCommandV3WithEmbed(
				[]string{"go", "build", "./..."},
				root,
				includes,
				output,
				true,
			)

			if code != tc.wantExit {
				t.Fatalf("capture exit = %d, want %d", code, tc.wantExit)
			}

			info, err := os.Stat(output)
			exists := err == nil && info.Size() > 0
			if exists != tc.wantCapsule {
				t.Fatalf("capsule exists = %v, want %v; stat: %v",
					exists, tc.wantCapsule, err)
			}

			if !exists {
				return
			}

			verified, err := capsule.ReadVerifiedV3(output)
			if err != nil {
				t.Fatalf("read verified capsule: %v", err)
			}

			expected := tc.embeddedCount + 3
			if len(verified.Files) != expected {
				t.Fatalf("archive files = %d, want %d (including virtual go.sum)",
					len(verified.Files), expected)
			}

			embedded := 0
			for _, file := range verified.Files {
				if len(file.Path) >= len("assets/") &&
					file.Path[:len("assets/")] == "assets/" {
					embedded++
				}
			}
			if embedded != tc.embeddedCount {
				t.Fatalf("embedded files = %d, want %d",
					embedded, tc.embeddedCount)
			}
		})
	}
}
