package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV3GoSourcesWithEmbedAndExplicitFiles(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"go.mod": "module example.com/source-compat\n\ngo 1.26.0\n",
		"go.sum": "",
		"main.go": `package main

import _ "embed"

//go:embed assets/message.txt
var message string

func main() {
	_ = message
	missingFunction()
}
`,
		"helper.go":          "package main\nfunc helper() {}\n",
		"assets/message.txt": "required embedded resource\n",
		"manual.txt":         "explicitly included data\n",
		".env":               "SECRET_DO_NOT_INCLUDE=123\n",
	}

	for name, content := range files {
		if err := os.WriteFile(
			filepath.Join(root, name),
			[]byte(content),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(root, "failure.bfc")

	got := captureCommandV3WithDiscovery(
		[]string{"go", "build", "./..."},
		root,
		[]string{"manual.txt", "main.go", "go.mod"},
		output,
		true,
		true,
	)

	if got != 1 {
		t.Fatalf("capture exit = %d, want 1", got)
	}

	verified, err := capsule.ReadVerifiedV3(output)
	if err != nil {
		t.Fatal(err)
	}

	found := make(map[string]bool)

	for _, file := range verified.Files {
		if found[file.Path] {
			t.Fatalf("duplicate captured path: %q", file.Path)
		}
		found[file.Path] = true
	}

	want := []string{
		"go.mod",
		"go.sum",
		"main.go",
		"helper.go",
		"assets/message.txt",
		"manual.txt",
	}

	for _, name := range want {
		if !found[name] {
			t.Errorf("missing captured file: %q", name)
		}
	}

	if found[".env"] {
		t.Fatal("unexpected .env in capsule")
	}

	if len(found) != len(want) {
		t.Fatalf("captured file count = %d, want %d: %v",
			len(found), len(want), found)
	}
}
