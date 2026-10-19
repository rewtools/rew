package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/execution"
)

func TestCaptureV4IdenticalBytesReplacementAudit(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")

	for _, dir := range []string{app, lib} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"),
		"module example.com/app\n\n"+
			"go 1.25\n\n"+
			"require example.com/local-lib v0.0.0\n\n"+
			"replace example.com/local-lib => ../lib\n",
	)

	write(filepath.Join(app, "main.go"),
		"package main\n\n"+
			"import _ \"example.com/local-lib\"\n\n"+
			"func main() {}\n",
	)

	write(filepath.Join(lib, "go.mod"),
		"module example.com/local-lib\n\ngo 1.25\n",
	)

	write(filepath.Join(lib, "lib.go"),
		"package lib\n\nfunc Broken( {\n",
	)

	target := filepath.Join(lib, "lib.go")
	output := filepath.Join(parent, "failure.rew")

	calls := 0

	executor := func(
		argv []string,
		executable string,
		dir string,
		env []string,
		stdout, stderr io.Writer,
	) (execution.Result, error) {
		calls++

		if calls != 1 {
			t.Fatal("executor called more than once")
		}

		originalInfo, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}

		original, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}

		replacement := target + ".replacement"

		if err := os.WriteFile(replacement, original, 0600); err != nil {
			t.Fatal(err)
		}

		if err := os.Rename(replacement, target); err != nil {
			t.Fatal(err)
		}

		newInfo, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}

		if os.SameFile(originalInfo, newInfo) {
			t.Fatal("file identity did not change")
		}

		current, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(original, current) {
			t.Fatal("replacement bytes differ")
		}

		t.Log("confirmed: different file identity, identical bytes")

		return execution.Result{
			Argv:     append([]string(nil), argv...),
			ExitCode: 1,
			Stderr:   "simulated Go build failure\n",
		}, nil
	}

	code := captureCommandV4WithExecutor(
		[]string{"go", "build", "./..."},
		app,
		output,
		executor,
	)

	if calls != 1 {
		t.Fatalf("executor calls=%d, want 1", calls)
	}

	switch code {
	case 1:
		verified, err := capsule.ReadVerifiedV4(output)
		if err != nil {
			t.Fatalf("verify published capsule: %v", err)
		}

		if verified.Manifest.Execution.ExitCode != 1 {
			t.Fatal("incorrect recorded exit code")
		}

		t.Log("OBSERVED: identical-byte file replacement was accepted")

	case 125:
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Fatalf("rejected capture still published capsule: %v", err)
		}

		t.Log("OBSERVED: identical-byte file replacement was rejected")

	default:
		t.Fatalf("unexpected capture exit code: %d", code)
	}
}
