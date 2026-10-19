package replay

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestRunMixedV4RejectsTamperedExternalZIPBeforeDocker(t *testing.T) {
	verified := mixedSnapshotFixtureV4(t)

	var original bytes.Buffer
	if err := capsule.WriteWorkspaceFromStoreV4(
		&original,
		verified.Manifest,
		verified.Files,
		verified.Store,
	); err != nil {
		t.Fatalf("create valid capsule: %v", err)
	}

	validPath := filepath.Join(t.TempDir(), "valid.rew")
	if err := os.WriteFile(validPath, original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	validated, err := capsule.ReadVerifiedToStoreV4(validPath)
	if err != nil {
		t.Fatalf("control capsule must verify: %v", err)
	}
	if err := validated.Close(); err != nil {
		t.Fatalf("close control capsule: %v", err)
	}

	var tampered bytes.Buffer
	reader := tar.NewReader(bytes.NewReader(original.Bytes()))
	writer := tar.NewWriter(&tampered)

	mutated := 0

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read source TAR: %v", err)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read source entry %q: %v", header.Name, err)
		}

		if header.Name == "dependencies/module-0001.zip" {
			if len(data) == 0 {
				t.Fatal("empty external ZIP")
			}
			data[len(data)-1] ^= 0x01
			mutated++
		}

		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write TAR header: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write TAR entry: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close tampered TAR: %v", err)
	}
	if mutated != 1 {
		t.Fatalf("mutated ZIP entries=%d, want 1", mutated)
	}

	capsulePath := filepath.Join(t.TempDir(), "tampered.rew")
	if err := os.WriteFile(capsulePath, tampered.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	t.Setenv(
		"DOCKER_HOST",
		"unix:///nonexistent/rew-tamper-test/docker.sock",
	)

	_, err = RunMixedV4WithOptions(
		context.Background(),
		capsulePath,
		Options{
			AllowArbitraryCommand: true,
			MemoryBytes:           512 << 20,
		},
	)

	if err == nil {
		t.Fatal("tampered external ZIP was accepted")
	}

	if !strings.Contains(err.Error(), "verify mixed v4 capsule") ||
		!strings.Contains(err.Error(), "verify stored mixed v4 artifact") {
		t.Fatalf("wrong rejection stage: %v", err)
	}

	if strings.Contains(err.Error(), "Docker") ||
		strings.Contains(err.Error(), "docker SDK") {
		t.Fatalf("unexpected Docker-stage failure: %v", err)
	}

	rejectionErr := err
	remaining, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf(
			"tampered capsule leaked %d temporary entries",
			len(remaining),
		)
	}

	t.Logf("tampered external ZIP rejected before Docker: %v", rejectionErr)
}
