package docker

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMixedV4OfflineDockerE2E(t *testing.T) {
	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Skip("set REW_DOCKER_TEST=1")
	}
	if os.Geteuid() == 0 {
		t.Skip("requires non-root host UID")
	}

	root := t.TempDir()
	t.Cleanup(func() {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			if entry.Type().IsRegular() {
				return os.Chmod(path, 0600)
			}
			return nil
		})
		if err != nil {
			t.Errorf("restore test module cache permissions: %v", err)
		}
	})
	proxy := filepath.Join(root, "proxy", "example.com", "external", "@v")
	if err := os.MkdirAll(proxy, 0700); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(proxy, "v1.0.0.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	for name, content := range map[string]string{
		"example.com/external@v1.0.0/go.mod":      "module example.com/external\n\ngo 1.25\n",
		"example.com/external@v1.0.0/external.go": "package external\nfunc Hello() {}\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(proxy, "v1.0.0.mod"),
		[]byte("module example.com/external\n\ngo 1.25\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(proxy, "v1.0.0.info"),
		[]byte(`{"Version":"v1.0.0","Time":"2026-01-01T00:00:00Z"}`),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	mainMod := "module example.com/app\n\ngo 1.25\n\n" +
		"require (\n" +
		"example.com/external v1.0.0\n" +
		"example.com/local-lib v0.0.0\n" +
		")\n\n" +
		"replace example.com/local-lib => ../lib\n"
	if err := os.WriteFile(
		filepath.Join(app, "go.mod"), []byte(mainMod), 0600,
	); err != nil {
		t.Fatal(err)
	}

	sourceCache := filepath.Join(root, "source-cache")
	cmd := exec.Command(
		"go", "mod", "download", "example.com/external@v1.0.0",
	)
	cmd.Dir = app
	cmd.Env = append(os.Environ(),
		"GOPROXY=file://"+filepath.Join(root, "proxy"),
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"GOMODCACHE="+sourceCache,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare local Go artifact: %v\n%s", err, output)
	}

	goSum, err := os.ReadFile(filepath.Join(app, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}

	workspaceFiles := map[string]string{
		"app/go.mod": mainMod,
		"app/go.sum": string(goSum),
		"app/main.go": "package main\n" +
			"import _ \"example.com/external\"\n" +
			"import _ \"example.com/local-lib\"\n" +
			"func main() {}\n",
		"lib/go.mod": "module example.com/local-lib\n\ngo 1.25\n",
		"lib/lib.go": "package local\nfunc Broken( {\n",
	}

	makeTAR := func(files map[string][]byte) []byte {
		t.Helper()
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for name, data := range files {
			if err := tw.WriteHeader(&tar.Header{
				Name: name, Mode: 0644,
				Size: int64(len(data)), Typeflag: tar.TypeReg,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	ws := make(map[string][]byte)
	for name, data := range workspaceFiles {
		ws[name] = []byte(data)
	}
	workspaceTAR := makeTAR(ws)

	modules := make(map[string][]byte)
	for _, ext := range []string{".zip", ".mod", ".info"} {
		name := "v1.0.0" + ext
		data, err := os.ReadFile(filepath.Join(
			sourceCache, "cache", "download",
			"example.com", "external", "@v", name,
		))
		if err != nil {
			t.Fatal(err)
		}
		modules["cache/download/example.com/external/@v/"+name] = data
	}
	moduleTAR := makeTAR(modules)

	ctx, cancel := context.WithTimeout(
		context.Background(), 6*time.Minute,
	)
	defer cancel()

	user := fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid())
	var stdout, stderr bytes.Buffer

	code, err := RunWithSDKRuntimeSnapshotExecReadersMixedV4(
		ctx,
		bytes.NewReader(workspaceTAR),
		bytes.NewReader(moduleTAR),
		[]string{"go", "build", "./..."},
		"example.com/app",
		"example.com/local-lib",
		"example.com/external",
		"v1.0.0",
		512<<20,
		user,
		&stdout,
		&stderr,
		&RuntimeSpec{Kind: "go", Version: "1.26.6"},
	)
	if err != nil {
		t.Fatalf("mixed Docker preparation/graph failed: %v\nstderr: %s",
			err, stderr.String())
	}

	t.Logf("exit code: %s", strconv.Itoa(code))
	t.Logf("stderr:\n%s", stderr.String())

	if code == 0 {
		t.Fatal("expected captured build to fail")
	}
	if !strings.Contains(stderr.String(), "lib.go") {
		t.Fatalf("expected local module compilation failure: %s",
			stderr.String())
	}

	t.Run("missing external ZIP prevents execution", func(t *testing.T) {
		withoutZIP := make(map[string][]byte, len(modules))
		for name, data := range modules {
			if strings.HasSuffix(name, ".zip") {
				continue
			}
			withoutZIP[name] = data
		}
		if len(withoutZIP) != 2 {
			t.Fatal("expected only .mod and .info artifacts")
		}

		var blockedStdout, blockedStderr bytes.Buffer

		_, blockedErr := RunWithSDKRuntimeSnapshotExecReadersMixedV4(
			ctx,
			bytes.NewReader(workspaceTAR),
			bytes.NewReader(makeTAR(withoutZIP)),
			[]string{
				"/bin/sh", "-c",
				"echo REW_UNEXPECTED_EXECUTION",
			},
			"example.com/app",
			"example.com/local-lib",
			"example.com/external",
			"v1.0.0",
			512<<20,
			user,
			&blockedStdout,
			&blockedStderr,
			&RuntimeSpec{Kind: "go", Version: "1.26.6"},
		)

		if blockedErr == nil {
			t.Fatal("missing external ZIP did not fail closed")
		}
		if !strings.Contains(
			blockedErr.Error(),
			"mixed v4 offline module preparation failed",
		) {
			t.Fatalf("unexpected failure stage: %v", blockedErr)
		}

		combined := blockedStdout.String() +
			blockedStderr.String() +
			blockedErr.Error()

		if strings.Contains(combined, "REW_UNEXPECTED_EXECUTION") {
			t.Fatal("user command executed despite missing ZIP")
		}
		if blockedStdout.Len() != 0 || blockedStderr.Len() != 0 {
			t.Fatalf(
				"user output appeared before verified graph: stdout=%q stderr=%q",
				blockedStdout.String(),
				blockedStderr.String(),
			)
		}

		t.Logf("missing ZIP correctly rejected: %v", blockedErr)
	})
}
