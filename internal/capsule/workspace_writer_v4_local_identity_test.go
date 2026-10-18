package capsule

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func validWorkspaceWriterFilesV4() []WorkspaceFile {
	return []WorkspaceFile{
		{
			Path: "app/go.mod",
			Data: []byte(`module example.com/app

go 1.25

require example.com/local-lib v0.0.0

replace example.com/local-lib => ../lib
`),
			Mode: 0600,
		},
		{
			Path: "app/main.go",
			Data: []byte("package main\nfunc main() {}\n"),
			Mode: 0600,
		},
		{
			Path: "lib/go.mod",
			Data: []byte("module example.com/local-lib\n\ngo 1.25\n"),
			Mode: 0600,
		},
		{
			Path: "lib/lib.go",
			Data: []byte("package local\nfunc Hello() {}\n"),
			Mode: 0600,
		},
	}
}

func TestWriteWorkspaceV4RejectsWrongLocalGoModBeforeOutput(t *testing.T) {
	manifest := validLocalReplaceManifestV4()
	files := validWorkspaceWriterFilesV4()

	files[2].Data = []byte("module example.com/unrelated\n\ngo 1.25\n")

	var output bytes.Buffer
	err := WriteWorkspaceV4(&output, manifest, files)

	if err == nil {
		t.Fatal("expected local go.mod identity rejection")
	}
	if !strings.Contains(err.Error(), "local module identity") {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("invalid input wrote %d bytes", output.Len())
	}
}

type failingWriterV4 struct{}

var errWriterFailureV4 = errors.New("intentional writer failure")

func (failingWriterV4) Write([]byte) (int, error) {
	return 0, errWriterFailureV4
}

func TestWriteWorkspaceV4PropagatesOutputFailure(t *testing.T) {
	manifest := validLocalReplaceManifestV4()
	files := validWorkspaceWriterFilesV4()

	err := WriteWorkspaceV4(failingWriterV4{}, manifest, files)
	if err == nil {
		t.Fatal("expected output failure")
	}
	if !errors.Is(err, errWriterFailureV4) {
		t.Fatalf("writer error was not preserved: %v", err)
	}
}
