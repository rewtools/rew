package capsule

import (
	"bytes"
	"testing"
)

func TestWriteWorkspaceV4RejectsFalseModuleIdentity(t *testing.T) {
	manifest := validLocalReplaceManifestV4()

	// Deliberately construct internally consistent metadata that
	// disagrees with the actual captured Go module declarations.
	manifest.GoSelectedModules[0].Path = "example.com/incorrect"
	manifest.LocalModules[0].ModulePath = "example.com/incorrect"

	files := []WorkspaceFile{
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
			Data: []byte(`module example.com/local-lib

go 1.25
`),
			Mode: 0600,
		},
		{
			Path: "lib/lib.go",
			Data: []byte("package local\n"),
			Mode: 0600,
		},
	}

	var output bytes.Buffer

	err := WriteWorkspaceV4(&output, manifest, files)

	if err == nil {
		t.Fatal("expected rejection of false selected module identity")
	}
	if output.Len() != 0 {
		t.Fatalf(
			"writer emitted %d bytes despite invalid module identity",
			output.Len(),
		)
	}
}
