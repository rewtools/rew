package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureUsageStderr(t *testing.T, runCommand func() int) (int, string) {
	t.Helper()

	oldStderr := os.Stderr

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stderr = writer
	defer func() {
		os.Stderr = oldStderr
	}()

	code := runCommand()

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	return code, string(output)
}

func TestCapsuleExtensionRuntimeUsage(t *testing.T) {
	t.Run("audit", func(t *testing.T) {
		code, output := captureUsageStderr(t, func() int {
			return runAudit(nil)
		})

		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}

		if !strings.Contains(output, "rew audit <capsule.rew>") {
			t.Fatalf("audit usage missing .rew:\n%s", output)
		}
	})

	t.Run("replay", func(t *testing.T) {
		code, output := captureUsageStderr(t, func() int {
			return runReplay(nil)
		})

		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}

		for _, expected := range []string{
			"rew replay [--allow-command] <capsule.bfc>",
			"rew replay --v2 --allow-command",
			"[--show-stderr] <capsule.rew>",
		} {
			if !strings.Contains(output, expected) {
				t.Errorf("replay usage missing %q:\n%s", expected, output)
			}
		}
	})
}
