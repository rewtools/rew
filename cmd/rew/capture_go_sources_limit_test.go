package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestCaptureV3GoSourcesReservesVirtualGoSum(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/source-limit\n\ngo 1.26.0\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	// Source files plus go.mod exactly fill the workspace limit.
	// Capture must reserve one additional slot for virtual go.sum.
	sourceCount := capsule.MaxWorkspaceV3Files - 1

	for i := 0; i < sourceCount; i++ {
		name := fmt.Sprintf("source_%04d.go", i)
		content := fmt.Sprintf(
			"package main\nvar source%d = %d\n",
			i, i,
		)

		if i == 0 {
			content += "func main() { undefinedFunction() }\n"
		}

		if err := os.WriteFile(
			filepath.Join(root, name),
			[]byte(content),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(root, "failure.bfc")

	got, stderr := captureUsageStderr(t, func() int {
		return captureCommandV3WithDiscovery(
			[]string{"go", "build", "./..."},
			root,
			nil,
			output,
			false,
			true,
		)
	})

	if got != 125 {
		t.Fatalf("capture exit = %d, want 125; stderr=%q", got, stderr)
	}

	want := fmt.Sprintf(
		"schema-v3 workspace file limit leaves no room for virtual go.sum (maximum %d files)",
		capsule.MaxWorkspaceV3Files,
	)
	if !strings.Contains(stderr, want) {
		t.Fatalf("missing limit diagnostic %q in stderr: %q", want, stderr)
	}
	if strings.Contains(stderr, "%d") {
		t.Fatalf("unexpanded format placeholder in stderr: %q", stderr)
	}

	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected capsule: %v", err)
	}
}

func TestCaptureV3GoSourcesCombinedFileLimit(t *testing.T) {
	setupCaptureV3WithoutCredentials(t)
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")

	root := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/combined-source-limit\n\ngo 1.26.0\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < capsule.MaxWorkspaceV3Files-1; i++ {
		name := fmt.Sprintf("source_%04d.go", i)
		content := fmt.Sprintf("package main\nvar source%d = %d\n", i, i)
		if i == 0 {
			content += "func main() {}\n"
		}

		if err := os.WriteFile(
			filepath.Join(root, name), []byte(content), 0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	extras := []string{"extra-one.txt", "extra-two.txt"}
	for _, name := range extras {
		if err := os.WriteFile(
			filepath.Join(root, name), []byte("test fixture\n"), 0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(root, "failure.rew")

	code, stderr := captureUsageStderr(t, func() int {
		return captureCommandV3WithDiscovery(
			[]string{"go", "build", "./..."},
			root,
			extras,
			output,
			false,
			true,
		)
	})

	if code != 125 {
		t.Fatalf("capture exit = %d, want 125; stderr=%q", code, stderr)
	}

	want := fmt.Sprintf(
		"schema-v3 workspace file limit exceeded (maximum %d files)",
		capsule.MaxWorkspaceV3Files,
	)
	if !strings.Contains(stderr, want) {
		t.Fatalf("missing combined file limit diagnostic %q: %q", want, stderr)
	}
	if strings.Contains(stderr, "%d") {
		t.Fatalf("unexpanded format placeholder: %q", stderr)
	}

	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected capsule: %v", err)
	}
}
