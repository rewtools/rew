package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV3GoSourcesCLI(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/source-cli\n\ngo 1.26.0\n",
		"main.go": `package main

func main() {
	undefinedFunction()
}
`,
		"helper.go": `package main

func helper() {}
`,
		".env":             "PRIVATE_VALUE=must-not-be-captured\n",
		"private-note.txt": "not a Go build input\n",
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

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	got := run([]string{
		"capture",
		"--v3",
		"--include-go-sources",
		"--",
		"go", "build", "./...",
	})

	if got != 1 {
		t.Fatalf("capture exit = %d, want 1", got)
	}

	output := filepath.Join(root, "failure.bfc")

	verified, err := capsule.ReadVerifiedV3(output)
	if err != nil {
		t.Fatalf("verify captured capsule: %v", err)
	}

	found := make(map[string]bool)
	for _, file := range verified.Files {
		found[file.Path] = true

		if strings.Contains(string(file.Data), "PRIVATE_VALUE") ||
			strings.Contains(string(file.Data), "not a Go build input") {
			t.Fatalf("unexpected private content in capsule: %q", file.Path)
		}
	}

	for _, name := range []string{
		"go.mod", "go.sum", "main.go", "helper.go",
	} {
		if !found[name] {
			t.Fatalf("missing automatically included file: %q", name)
		}
	}

	if len(found) != 4 {
		t.Fatalf("captured files = %v, want exactly 4", found)
	}
}
