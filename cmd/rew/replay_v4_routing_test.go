package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayV4RoutesToVerifiedReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.rew")

	if err := os.WriteFile(
		path,
		[]byte("not a valid schema v4 capsule"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer

	code := runReplay([]string{
		"--v4", "--allow-command", path,
	})

	os.Stderr = oldStderr
	writer.Close()

	output, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	if code != 125 {
		t.Fatalf("exit code=%d, want 125", code)
	}

	if !strings.Contains(
		string(output), "verify v4 capsule",
	) {
		t.Fatalf(
			"CLI did not use V4 reader: %s",
			output,
		)
	}
}

func TestReplayV4RejectsMissingOptIn(t *testing.T) {
	code := runReplay([]string{
		"--v4", "failure.rew",
	})

	if code != 2 {
		t.Fatalf("exit code=%d, want 2", code)
	}
}

func TestReplayV4RejectsInvalidMemory(t *testing.T) {
	code := runReplay([]string{
		"--v4",
		"--allow-command",
		"--memory",
		"3g",
		"failure.rew",
	})

	if code != 2 {
		t.Fatalf("exit code=%d, want 2", code)
	}
}
