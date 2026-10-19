package capsule

import (
	"archive/tar"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

// WriteWorkspaceSnapshotV4 converts verified schema-v4 workspace
// data into a TAR whose entries are relative to the Docker volume.
//
// This function does not extract or execute archived content.
// Input errors are rejected before emitting any bytes.
// An output write failure may still leave a partial TAR.
func WriteWorkspaceSnapshotV4(
	w io.Writer,
	verified VerifiedCapsuleV4,
) error {
	if w == nil {
		return fmt.Errorf("capsule: nil v4 snapshot writer")
	}

	manifest := verified.Manifest

	if manifest.SchemaVersion != SchemaVersionV4 {
		return fmt.Errorf("capsule: snapshot requires schema v4")
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("capsule: invalid v4 snapshot manifest: %w", err)
	}
	if len(manifest.GoModules) != 0 ||
		len(manifest.GoGraphInfoFiles) != 0 ||
		len(manifest.GoGraphModFiles) != 0 {
		return fmt.Errorf(
			"capsule: v4 snapshot does not support external artifacts",
		)
	}
	if len(manifest.LocalModules) != 1 {
		return fmt.Errorf("capsule: v4 snapshot requires one local module")
	}
	if len(verified.Files) != len(manifest.Workspace.Files) {
		return fmt.Errorf("capsule: v4 snapshot file count mismatch")
	}

	expected := make(
		map[string]FileMetadata,
		len(manifest.Workspace.Files),
	)
	for _, metadata := range manifest.Workspace.Files {
		if _, exists := expected[metadata.Path]; exists {
			return fmt.Errorf(
				"capsule: duplicate v4 snapshot metadata: %q",
				metadata.Path,
			)
		}
		expected[metadata.Path] = metadata
	}

	files := append([]WorkspaceFile(nil), verified.Files...)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	mainRoot := manifest.Execution.WorkingDir
	localRoot := manifest.LocalModules[0].SnapshotPath

	seen := make(map[string]bool, len(files))
	var totalSize int64
	var mainGoMod, localGoMod []byte
	foundMain, foundLocal := false, false

	var estimated int64 = 1024

	for _, file := range files {
		if err := ValidateWorkspacePath(file.Path, file.Mode); err != nil {
			return fmt.Errorf(
				"capsule: invalid v4 snapshot path: %w",
				err,
			)
		}

		if seen[file.Path] {
			return fmt.Errorf(
				"capsule: duplicate v4 snapshot file: %q",
				file.Path,
			)
		}
		seen[file.Path] = true

		isMain := strings.HasPrefix(file.Path, mainRoot+"/")
		isLocal := strings.HasPrefix(file.Path, localRoot+"/")
		if !isMain && !isLocal {
			return fmt.Errorf(
				"capsule: file outside v4 snapshot roots: %q",
				file.Path,
			)
		}

		metadata, ok := expected[file.Path]
		if !ok {
			return fmt.Errorf(
				"capsule: unexpected v4 snapshot file: %q",
				file.Path,
			)
		}
		if err := VerifyWorkspaceFileV2(
			metadata,
			file.Data,
		); err != nil {
			return fmt.Errorf(
				"capsule: invalid v4 snapshot file %q: %w",
				file.Path,
				err,
			)
		}

		size := int64(len(file.Data))
		if size > MaxWorkspaceFileSize ||
			size > MaxWorkspaceV2TotalSize-totalSize {
			return fmt.Errorf("capsule: v4 snapshot workspace size exceeded")
		}
		totalSize += size

		estimated += tarEntrySize(size)
		if estimated > MaxCapsuleSizeV3 {
			return fmt.Errorf("capsule: v4 snapshot TAR size exceeded")
		}

		if file.Path == path.Join(mainRoot, "go.mod") {
			mainGoMod = file.Data
			foundMain = true
		}
		if file.Path == path.Join(localRoot, "go.mod") {
			localGoMod = file.Data
			foundLocal = true
		}
	}

	if !foundMain || !foundLocal {
		return fmt.Errorf("capsule: missing v4 snapshot go.mod")
	}

	for name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf(
					"capsule: v4 snapshot file/directory conflict: %q",
					parent,
				)
			}
		}
	}

	if err := ValidateGoModuleConsistencyV4(
		mainGoMod,
		manifest.GoSelectedModules,
	); err != nil {
		return fmt.Errorf(
			"capsule: inconsistent v4 snapshot module graph: %w",
			err,
		)
	}

	localParsed, err := modfile.Parse(
		path.Join(localRoot, "go.mod"),
		localGoMod,
		nil,
	)
	if err != nil {
		return fmt.Errorf("capsule: parse v4 snapshot local go.mod: %w", err)
	}
	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path != manifest.LocalModules[0].ModulePath {
		return fmt.Errorf(
			"capsule: v4 snapshot local module identity mismatch",
		)
	}

	writeTAR := func(dst io.Writer) error {
		tw := tar.NewWriter(dst)

		for _, file := range files {
			header := &tar.Header{
				Name:     file.Path,
				Mode:     0644,
				Size:     int64(len(file.Data)),
				Typeflag: tar.TypeReg,
			}

			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf(
					"capsule: write v4 snapshot header %q: %w",
					file.Path, err,
				)
			}
			if _, err := tw.Write(file.Data); err != nil {
				return fmt.Errorf(
					"capsule: write v4 snapshot data %q: %w",
					file.Path, err,
				)
			}
		}

		if err := tw.Close(); err != nil {
			return fmt.Errorf("capsule: close v4 snapshot TAR: %w", err)
		}
		return nil
	}

	// Serialize once to a bounded discard sink so malformed
	// input or TAR header failures cannot publish partial data.
	preflight := &workspaceSnapshotLimitWriterV3{
		dst:       io.Discard,
		remaining: MaxCapsuleSizeV3,
	}
	if err := writeTAR(preflight); err != nil {
		return fmt.Errorf("capsule: v4 snapshot preflight: %w", err)
	}

	limited := &workspaceSnapshotLimitWriterV3{
		dst:       w,
		remaining: MaxCapsuleSizeV3,
	}
	if err := writeTAR(limited); err != nil {
		return fmt.Errorf("capsule: write v4 snapshot: %w", err)
	}

	return nil
}
