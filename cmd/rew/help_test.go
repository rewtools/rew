package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func runHelpTest(args []string) (int, string, string) {
	oldOut := os.Stdout
	oldErr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()

	os.Stdout = wOut
	os.Stderr = wErr

	defer func() {
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	code := run(args)

	wOut.Close()
	wErr.Close()

	out, _ := io.ReadAll(rOut)
	errOut, _ := io.ReadAll(rErr)

	rOut.Close()
	rErr.Close()

	return code, string(out), string(errOut)
}

func TestCLIHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"general", []string{"--help"}, "Commands:"},
		{"general short", []string{"-h"}, "Commands:"},
		{"help command", []string{"help"}, "Commands:"},
		{"capture", []string{"capture", "--help"}, "go build ./..."},
		{"capture short", []string{"capture", "-h"}, "go build ./..."},
		{"replay", []string{"replay", "--help"}, "--memory"},
		{"replay short", []string{"replay", "-h"}, "--memory"},
		{"audit", []string{"audit", "--help"}, "safe to share"},
		{"audit short", []string{"audit", "-h"}, "safe to share"},
		{"version alias", []string{"--version"}, "rew dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runHelpTest(tt.args)

			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("stdout missing %q: %q", tt.want, stdout)
			}
			if stderr != "" {
				t.Fatalf("unexpected stderr: %q", stderr)
			}
		})
	}
}

func TestCLIHelpInvalidArguments(t *testing.T) {
	tests := [][]string{
		{"--help", "extra"},
		{"--version", "extra"},
		{"help", "extra"},
		{"capture", "--help", "extra"},
		{"replay", "--help", "extra"},
		{"audit", "--help", "extra"},
		{"unknown-command"},
	}

	for _, args := range tests {
		code, _, _ := runHelpTest(args)

		if code != 2 {
			t.Errorf("args %v: exit code = %d, want 2", args, code)
		}
	}
}
