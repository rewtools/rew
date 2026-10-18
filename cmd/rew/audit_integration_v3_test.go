package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func makeAuditCapsuleV3(t *testing.T) string {
	t.Helper()

	manifest := capsule.Manifest{
		SchemaVersion: capsule.SchemaVersionV3,
		Runtime: &capsule.Runtime{
			Kind:    "go",
			Version: capsule.SupportedGoVersion,
		},
		GoBuildEnv: &capsule.GoBuildEnvironment{
			GOOS:       "linux",
			GOARCH:     "amd64",
			CGOEnabled: "0",
		},
		Platform: capsule.Platform{
			OS:           "darwin",
			Architecture: "arm64",
		},
		Execution: capsule.Execution{
			Argv:       []string{"go", "build", "./..."},
			WorkingDir: ".",
			ExitCode:   1,
			Stderr:     "undefined: missingSymbol\n",
		},
	}

	files := []capsule.WorkspaceFile{
		{
			Path: "go.mod",
			Data: []byte(
				"module example.com/audit-fixture\n\ngo 1.26.6\n",
			),
			Mode: 0600,
		},
		{
			Path: "go.sum",
			Data: nil,
			Mode: 0600,
		},
		{
			Path: "main.go",
			Data: []byte(
				"package main\nfunc main() { missingSymbol() }\n",
			),
			Mode: 0600,
		},
	}

	var archive bytes.Buffer
	if err := capsule.WriteWorkspaceV3(
		&archive,
		manifest,
		files,
		nil,
	); err != nil {
		t.Fatalf("create audit fixture: %v", err)
	}

	filename := filepath.Join(t.TempDir(), "audit-fixture.bfc")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	return filename
}

func TestAuditV3ValidCapsuleReadOnly(t *testing.T) {
	filename := makeAuditCapsuleV3(t)

	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	code, output := captureAuditCLIOutput(t, []string{
		"audit", filename,
	})

	if code != 0 {
		t.Fatalf("expected exit 0, got %d:\n%s", code, output)
	}

	for _, expected := range []string{
		"Capsule integrity: verified",
		"Schema version: 3",
		"Workspace files: 3",
		"Go modules: 0",
		"Sensitive data findings: 0",
		"does not establish",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in output:\n%s", expected, output)
		}
	}

	after, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(before, after) {
		t.Fatal("audit modified capsule contents")
	}
}

func TestAuditV3RejectsCorruptCapsule(t *testing.T) {
	filename := makeAuditCapsuleV3(t)

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt an archived source file while leaving its manifest digest
	// unchanged. The verified reader must reject the mismatch.
	needle := []byte("missingSymbol()")
	index := bytes.Index(data, needle)
	if index < 0 {
		t.Fatal("fixture source marker not found")
	}

	data[index] ^= 1

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}

	code, output := captureAuditCLIOutput(t, []string{
		"audit", filename,
	})

	if code != 125 {
		t.Fatalf("expected exit 125, got %d:\n%s", code, output)
	}

	if !strings.Contains(output, "capsule verification failed") {
		t.Fatalf("missing verification error:\n%s", output)
	}

	if strings.Contains(output, "Capsule integrity: verified") {
		t.Fatal("corrupted capsule reported as verified")
	}

	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "rew-artifacts-v3-") {
			t.Fatalf("artifact store remained after failed verification: %q",
				entry.Name())
		}
	}
}

func TestAuditV3DoesNotExecuteRecordedCommand(t *testing.T) {
	filename := makeAuditCapsuleV3(t)
	marker := filepath.Join(t.TempDir(), "execution-marker")

	// A reader-only audit must not start Go, Docker or replay.
	// Deliberately remove PATH for the process during this test:
	// capsule verification must not require external executables.
	t.Setenv("PATH", "")
	t.Setenv("REW_AUDIT_TEST_MARKER", marker)

	code, output := captureAuditCLIOutput(t, []string{
		"audit", filename,
	})

	if code != 0 {
		t.Fatalf("audit unexpectedly required an executable: %d:\n%s",
			code, output)
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unexpected execution marker: %v", err)
	}
}

func TestAuditV3CleansUpTemporaryArtifacts(t *testing.T) {
	filename := makeAuditCapsuleV3(t)

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	code, output := captureAuditCLIOutput(t, []string{
		"audit", filename,
	})

	if code != 0 {
		t.Fatalf("expected successful audit, got %d:\n%s", code, output)
	}

	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "rew-artifacts-v3-") {
			t.Fatalf("temporary artifact store was not removed: %q",
				entry.Name())
		}
	}
}

func TestAuditV3DetectsSensitiveStderrWithoutLeakingValue(t *testing.T) {
	const secret = "synthetic-audit-secret-never-print"

	manifest := capsule.Manifest{
		SchemaVersion: capsule.SchemaVersionV3,
		Runtime: &capsule.Runtime{
			Kind:    "go",
			Version: capsule.SupportedGoVersion,
		},
		GoBuildEnv: &capsule.GoBuildEnvironment{
			GOOS:       "linux",
			GOARCH:     "amd64",
			CGOEnabled: "0",
		},
		Platform: capsule.Platform{
			OS:           "darwin",
			Architecture: "arm64",
		},
		Execution: capsule.Execution{
			Argv:       []string{"go", "build", "./..."},
			WorkingDir: ".",
			ExitCode:   1,
			Stderr:     "build error: GITHUB_TOKEN=" + secret,
		},
	}

	files := []capsule.WorkspaceFile{
		{
			Path: "go.mod",
			Data: []byte("module example.com/audit\n\ngo 1.26.6\n"),
			Mode: 0600,
		},
		{
			Path: "go.sum",
			Data: nil,
			Mode: 0600,
		},
		{
			Path: "main.go",
			Data: []byte("package main\nfunc main() { missingSymbol() }\n"),
			Mode: 0600,
		},
	}

	var archive bytes.Buffer
	if err := capsule.WriteWorkspaceV3(&archive, manifest, files, nil); err != nil {
		t.Fatal(err)
	}

	filename := filepath.Join(t.TempDir(), "sensitive.bfc")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	code, output := captureAuditCLIOutput(t, []string{"audit", filename})

	if code != 3 {
		t.Fatalf("expected exit 3, got %d:\n%s", code, output)
	}

	if !strings.Contains(output, "Finding: stderr: credential assignment") {
		t.Fatalf("missing stderr finding:\n%s", output)
	}

	if strings.Contains(output, secret) {
		t.Fatal("audit output leaked the synthetic credential")
	}
}

// A credential assignment inside an ordinary source file must
// be detected without exposing the credential value.
func TestAuditV3DetectsSourceContentCredential(t *testing.T) {
	const secret = "synthetic-source-token-never-print"

	manifest := capsule.Manifest{
		SchemaVersion: capsule.SchemaVersionV3,
		Runtime: &capsule.Runtime{
			Kind:    "go",
			Version: capsule.SupportedGoVersion,
		},
		GoBuildEnv: &capsule.GoBuildEnvironment{
			GOOS:       "linux",
			GOARCH:     "amd64",
			CGOEnabled: "0",
		},
		Platform: capsule.Platform{
			OS:           "darwin",
			Architecture: "arm64",
		},
		Execution: capsule.Execution{
			Argv:       []string{"go", "build", "./..."},
			WorkingDir: ".",
			ExitCode:   1,
			Stderr:     "undefined: missingSymbol\n",
		},
	}

	files := []capsule.WorkspaceFile{
		{
			Path: "go.mod",
			Data: []byte("module example.com/audit\n\ngo 1.26.6\n"),
			Mode: 0600,
		},
		{
			Path: "go.sum",
			Data: nil,
			Mode: 0600,
		},
		{
			Path: "main.go",
			Data: []byte(
				"package main\n" +
					"const token = \"GITHUB_TOKEN=" + secret + "\"\n" +
					"func main() { missingSymbol() }\n",
			),
			Mode: 0600,
		},
	}

	var archive bytes.Buffer
	if err := capsule.WriteWorkspaceV3(&archive, manifest, files, nil); err != nil {
		t.Fatal(err)
	}

	filename := filepath.Join(t.TempDir(), "source-token.bfc")
	if err := os.WriteFile(filename, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	code, output := captureAuditCLIOutput(t, []string{"audit", filename})

	if code != 3 {
		t.Fatalf("expected sensitive-content exit 3, got %d:\n%s",
			code, output)
	}

	if !strings.Contains(output, "workspace content: credential assignment") {
		t.Fatalf("missing workspace content finding:\n%s", output)
	}

	if strings.Contains(output, secret) {
		t.Fatal("audit output leaked synthetic source token")
	}
}
