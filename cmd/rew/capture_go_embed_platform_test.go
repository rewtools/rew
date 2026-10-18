package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV3GoEmbedPlatformResources(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOARCH", "arm64")

	root := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/embed-platform-capture\n\ngo 1.26\n",
		"main.go": `package main
func main() {
    rewIntentionallyUndefined()
}
`,
		"embed_darwin.go": `package main
import _ "embed"
//go:embed assets/darwin.txt
var darwinResource string
`,
		"embed_linux.go": `package main
import _ "embed"
//go:embed assets/linux.txt
var linuxResource string
`,
		"assets/darwin.txt": "macOS resource\n",
		"assets/linux.txt":  "Linux resource\n",
	}

	for name, data := range files {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(root, "failure.bfc")

	code := captureCommandV3WithEmbed(
		[]string{"go", "build", "./..."},
		root,
		[]string{
			"go.mod",
			"main.go",
			"embed_darwin.go",
			"embed_linux.go",
		},
		output,
		true,
	)

	if code != 1 {
		t.Fatalf("capture exit = %d, want 1", code)
	}

	verified, err := capsule.ReadVerifiedV3(output)
	if err != nil {
		t.Fatalf("verify capsule: %v", err)
	}

	expected := map[string]string{
		"assets/darwin.txt": "macOS resource\n",
		"assets/linux.txt":  "Linux resource\n",
	}

	for _, file := range verified.Files {
		want, ok := expected[file.Path]
		if !ok {
			continue
		}
		if !bytes.Equal(file.Data, []byte(want)) {
			t.Fatalf("wrong contents for %s", file.Path)
		}
		delete(expected, file.Path)
	}

	if len(expected) != 0 {
		t.Fatalf("missing embedded resources: %v", expected)
	}
}
