package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSnapshotExecutorV4LocalReplaceDockerE2E(t *testing.T) {
	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Skip("set REW_DOCKER_TEST=1 to enable Docker E2E")
	}

	files := []struct {
		name string
		data string
	}{
		{
			"app/go.mod",
			"module example.com/app\n\n" +
				"go 1.25\n\n" +
				"require example.com/local-lib v0.0.0\n\n" +
				"replace example.com/local-lib => ../lib\n",
		},
		{
			"app/main.go",
			"package main\n\n" +
				"import _ \"example.com/local-lib\"\n\n" +
				"func main() {}\n",
		},
		{
			"lib/go.mod",
			"module example.com/local-lib\n\ngo 1.25\n",
		},
		{
			"lib/lib.go",
			"package lib\n\n" +
				"func Broken( {\n",
		},
	}

	var workspace bytes.Buffer
	tw := tar.NewWriter(&workspace)

	for _, file := range files {
		data := []byte(file.data)
		err := tw.WriteHeader(&tar.Header{
			Name:     file.name,
			Mode:     0644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var modules bytes.Buffer
	mtw := tar.NewWriter(&modules)
	if err := mtw.Close(); err != nil {
		t.Fatal(err)
	}

	uid := os.Geteuid()
	if uid == 0 {
		t.Skip("Docker replay requires a non-root host UID")
	}
	user := strconv.Itoa(uid) + ":" + strconv.Itoa(os.Getegid())

	ctx, cancel := context.WithTimeout(
		context.Background(), 6*time.Minute,
	)
	defer cancel()

	var stdout, stderr bytes.Buffer

	code, err := RunWithSDKRuntimeSnapshotExecReadersV4(
		ctx,
		bytes.NewReader(workspace.Bytes()),
		bytes.NewReader(modules.Bytes()),
		[]string{"go", "build", "./..."},
		"example.com/app",
		"example.com/local-lib",
		512<<20,
		user,
		&stdout,
		&stderr,
		&RuntimeSpec{
			Kind:    "go",
			Version: "1.26.6",
		},
	)
	if err != nil {
		t.Fatalf(
			"V4 Docker executor failed before captured build: %v\nstderr:\n%s",
			err, stderr.String(),
		)
	}

	t.Logf("Docker exit code: %d", code)
	t.Logf("Docker stdout:\n%s", stdout.String())
	t.Logf("Docker stderr:\n%s", stderr.String())

	if code == 0 {
		t.Fatal("expected compilation failure from sibling lib")
	}
	if !strings.Contains(
		stderr.String(), "example.com/local-lib",
	) {
		t.Fatalf(
			"failure did not identify sibling module:\n%s",
			stderr.String(),
		)
	}
	if !strings.Contains(stderr.String(), "lib.go") {
		t.Fatalf(
			"failure did not identify lib.go:\n%s",
			stderr.String(),
		)
	}

	if _, err := io.Copy(io.Discard, bytes.NewReader(
		workspace.Bytes(),
	)); err != nil {
		t.Fatal(err)
	}
}
