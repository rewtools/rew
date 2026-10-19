package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureV4RejectsUnsupportedCommandsBeforeExecution(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{
			name: "empty",
			argv: nil,
		},
		{
			name: "go test",
			argv: []string{"go", "test", "./..."},
		},
		{
			name: "go build without target",
			argv: []string{"go", "build"},
		},
		{
			name: "alternate target",
			argv: []string{"go", "build", "."},
		},
		{
			name: "overlay",
			argv: []string{"go", "build", "-overlay=evil.json", "./..."},
		},
		{
			name: "modfile",
			argv: []string{"go", "build", "-modfile=other.mod", "./..."},
		},
		{
			name: "extra argument",
			argv: []string{"go", "build", "./...", "other"},
		},
		{
			name: "non Go command",
			argv: []string{"sh", "-c", "touch marker"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "failure.rew")
			marker := filepath.Join(dir, "marker")

			code := captureCommandV4(tc.argv, dir, output)

			if code != 125 {
				t.Fatalf("exit=%d, want 125", code)
			}

			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("capsule unexpectedly created: %v", err)
			}

			if _, err := os.Lstat(marker); !os.IsNotExist(err) {
				t.Fatalf("command unexpectedly executed: %v", err)
			}
		})
	}
}
