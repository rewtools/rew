package capsule

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestWriteWorkspaceV4TwoLocalModules(t *testing.T) {
	manifest := validLocalReplaceManifestV4()

	// The writer must derive these from the actual captured bytes.
	manifest.Workspace.Files = nil
	manifest.LocalModules[0].Files = nil

	files := []WorkspaceFile{
		{
			Path: "app/go.mod",
			Data: []byte(
				"module example.com/app\n\n" +
					"go 1.25\n\n" +
					"require example.com/local-lib v0.0.0\n\n" +
					"replace example.com/local-lib => ../lib\n",
			),
			Mode: 0600,
		},
		{
			Path: "app/main.go",
			Data: []byte(
				"package main\n\n" +
					"import \"example.com/local-lib\"\n\n" +
					"func main() { local.Hello() }\n",
			),
			Mode: 0600,
		},
		{
			Path: "lib/go.mod",
			Data: []byte(
				"module example.com/local-lib\n\n" +
					"go 1.25\n",
			),
			Mode: 0600,
		},
		{
			Path: "lib/lib.go",
			Data: []byte("package local\n\nfunc Hello() {}\n"),
			Mode: 0600,
		},
	}

	var output bytes.Buffer

	if err := WriteWorkspaceV4(&output, manifest, files); err != nil {
		t.Fatalf("write v4 capsule: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(output.Bytes()))

	header, err := tr.Next()
	if err != nil {
		t.Fatalf("read manifest entry: %v", err)
	}
	if header.Name != "manifest.json" ||
		header.Typeflag != tar.TypeReg {
		t.Fatalf("unexpected manifest header: %+v", header)
	}

	manifestBytes, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}

	var actual ManifestV4
	if err := json.Unmarshal(manifestBytes, &actual); err != nil {
		t.Fatal(err)
	}

	if err := actual.Validate(); err != nil {
		t.Fatalf("invalid written manifest: %v", err)
	}

	if len(actual.Workspace.Files) != len(files) {
		t.Fatalf(
			"workspace metadata count = %d, want %d",
			len(actual.Workspace.Files), len(files),
		)
	}

	if len(actual.LocalModules) != 1 ||
		len(actual.LocalModules[0].Files) != 2 {
		t.Fatalf("unexpected local module metadata: %+v",
			actual.LocalModules)
	}

	for i, file := range files {
		metadata := actual.Workspace.Files[i]
		if metadata.Path != file.Path ||
			metadata.Size != int64(len(file.Data)) ||
			metadata.SHA256 != SHA256(file.Data) {
			t.Fatalf("invalid metadata for %q: %+v",
				file.Path, metadata)
		}

		header, err := tr.Next()
		if err != nil {
			t.Fatalf("read workspace entry %d: %v", i, err)
		}
		if header.Name != "workspace/"+file.Path ||
			header.Typeflag != tar.TypeReg {
			t.Fatalf("unexpected workspace header: %+v", header)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, file.Data) {
			t.Fatalf("workspace content mismatch: %q", file.Path)
		}
	}

	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf("unexpected trailing TAR entry: %v", err)
	}

	for _, metadata := range actual.LocalModules[0].Files {
		if metadata.Path != "go.mod" && metadata.Path != "lib.go" {
			t.Fatalf("unexpected local file: %s", metadata.Path)
		}

		found := false
		for _, workspace := range actual.Workspace.Files {
			if workspace.Path != "lib/"+metadata.Path {
				continue
			}
			if workspace.Size != metadata.Size ||
				workspace.SHA256 != metadata.SHA256 {
				t.Fatalf(
					"local metadata mismatch: %s",
					metadata.Path,
				)
			}
			found = true
		}
		if !found {
			t.Fatalf("missing local workspace file: %s", metadata.Path)
		}
	}
}
