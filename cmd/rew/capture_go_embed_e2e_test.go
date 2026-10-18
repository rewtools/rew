package main

import (
	"bytes"
	"github.com/rewtools/rew/internal/capsule"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV3GoEmbedInclusion(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	cases := []struct {
		name         string
		includeEmbed bool
		explicitFile bool
		wantExit     int
		wantCapsule  bool
	}{
		{
			name:         "automatic resource inclusion",
			includeEmbed: true,
			wantExit:     1,
			wantCapsule:  true,
		},
		{
			name:         "explicit resource deduplicated",
			includeEmbed: true,
			explicitFile: true,
			wantExit:     1,
			wantCapsule:  true,
		},
		{
			name:        "legacy capture without auto embed",
			wantExit:    1,
			wantCapsule: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()

			if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
				t.Fatal(err)
			}

			files := map[string]string{
				"go.mod": "module example.com/embed-test\n\ngo 1.26\n",
				"main.go": `package main

import _ "embed"

//go:embed assets/message.txt
var message string

func main() {
    _ = message
    rewIntentionallyUndefined()
}
`,
				"assets/message.txt": "embedded test resource\n",
			}

			for name, content := range files {
				if err := os.WriteFile(
					filepath.Join(root, name),
					[]byte(content), 0600,
				); err != nil {
					t.Fatal(err)
				}
			}

			includes := []string{"go.mod", "main.go"}
			if tc.explicitFile {
				includes = append(includes, "assets/message.txt")
			}

			output := filepath.Join(root, "failure.bfc")

			got := captureCommandV3WithEmbed(
				[]string{"go", "build", "./..."},
				root,
				includes,
				output,
				tc.includeEmbed,
			)

			if got != tc.wantExit {
				t.Fatalf("capture exit = %d, want %d", got, tc.wantExit)
			}

			info, err := os.Stat(output)
			exists := err == nil && info.Size() > 0

			if exists != tc.wantCapsule {
				t.Fatalf(
					"capsule exists = %v, want %v; stat err = %v",
					exists, tc.wantCapsule, err,
				)
			}

			if !exists {
				return
			}

			verified, err := capsule.ReadVerifiedV3(output)
			if err != nil {
				t.Fatalf("read verified capsule: %v", err)
			}

			wantCount := 3
			wantEmbedded := 0
			if tc.includeEmbed {
				wantCount = 4
				wantEmbedded = 1
			}

			if len(verified.Files) != wantCount {
				t.Fatalf(
					"workspace file count = %d, want %d",
					len(verified.Files), wantCount,
				)
			}

			embeddedCount := 0
			for _, file := range verified.Files {
				if file.Path != "assets/message.txt" {
					continue
				}
				embeddedCount++
				if !bytes.Equal(file.Data, []byte("embedded test resource\n")) {
					t.Fatalf("embedded resource content mismatch: %q", file.Data)
				}
			}

			if embeddedCount != wantEmbedded {
				t.Fatalf(
					"embedded resource count = %d, want %d",
					embeddedCount, wantEmbedded,
				)
			}
		})
	}
}
