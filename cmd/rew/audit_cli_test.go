package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureAuditCLIOutput(t *testing.T, args []string) (int, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stdout = writer
	os.Stderr = writer

	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	code := run(args)

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	reader.Close()

	return code, string(output)
}

func TestAuditCLIInvalidArguments(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"missing capsule", []string{"audit"}},
		{"extra argument", []string{"audit", "a.bfc", "b.bfc"}},
		{"unsupported flag", []string{"audit", "--unknown"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, output := captureAuditCLIOutput(t, tc.args)

			if code != 2 {
				t.Fatalf("expected exit 2, got %d: %s", code, output)
			}

			if !strings.Contains(output, "Usage: rew audit") {
				t.Fatalf("missing audit usage: %s", output)
			}
		})
	}
}

func TestAuditCLIMissingCapsule(t *testing.T) {
	code, output := captureAuditCLIOutput(t, []string{
		"audit",
		"nonexistent-audit-fixture-12345.bfc",
	})

	if code != 125 {
		t.Fatalf("expected exit 125, got %d: %s", code, output)
	}

	if !strings.Contains(output, "rew: audit:") {
		t.Fatalf("missing audit error prefix: %s", output)
	}
}
