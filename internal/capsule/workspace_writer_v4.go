package capsule

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

// WriteWorkspaceV4 writes an experimental schema-v4 capsule containing
// one main Go module and one sibling local replacement module.
//
// External dependency artifacts are not supported by this initial writer.
// All input validation and TAR size preflight finish before writing to w.
// A destination write failure can still leave a partial archive.
func WriteWorkspaceV4(
	w io.Writer,
	manifest ManifestV4,
	files []WorkspaceFile,
) error {
	if w == nil {
		return fmt.Errorf("capsule: nil v4 writer")
	}
	if manifest.SchemaVersion != SchemaVersionV4 {
		return fmt.Errorf("capsule: v4 writer requires schema version 4")
	}
	if len(manifest.GoModules) != 0 ||
		len(manifest.GoGraphInfoFiles) != 0 ||
		len(manifest.GoGraphModFiles) != 0 {
		return fmt.Errorf(
			"capsule: v4 workspace-only writer does not support external artifacts",
		)
	}
	if len(manifest.LocalModules) != 1 {
		return fmt.Errorf("capsule: v4 writer requires one local module")
	}
	if len(files) == 0 || len(files) > MaxWorkspaceV3Files {
		return fmt.Errorf("capsule: invalid v4 workspace file count")
	}

	mainRoot := manifest.Execution.WorkingDir
	localRoot := manifest.LocalModules[0].SnapshotPath

	if err := validateV4SnapshotDirectory(mainRoot); err != nil {
		return fmt.Errorf("capsule: invalid main root: %w", err)
	}
	if err := validateV4SnapshotDirectory(localRoot); err != nil {
		return fmt.Errorf("capsule: invalid local root: %w", err)
	}
	if mainRoot == localRoot ||
		strings.HasPrefix(mainRoot, localRoot+"/") ||
		strings.HasPrefix(localRoot, mainRoot+"/") {
		return fmt.Errorf("capsule: overlapping v4 module roots")
	}

	ordered := append([]WorkspaceFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Path < ordered[j].Path
	})

	workspaceMetadata := make([]FileMetadata, 0, len(ordered))
	localMetadata := make([]FileMetadata, 0)
	seen := make(map[string]bool, len(ordered))
	var totalSize int64
	foundMainMod := false
	foundLocalMod := false
	var mainGoMod, localGoMod []byte

	for _, file := range ordered {
		if err := ValidateWorkspacePath(file.Path, file.Mode); err != nil {
			return fmt.Errorf("capsule: invalid v4 workspace file: %w", err)
		}
		if seen[file.Path] {
			return fmt.Errorf("capsule: duplicate v4 workspace file: %s", file.Path)
		}
		seen[file.Path] = true

		if int64(len(file.Data)) > MaxWorkspaceFileSize {
			return fmt.Errorf("capsule: v4 workspace file too large: %s", file.Path)
		}
		if int64(len(file.Data)) > MaxWorkspaceV2TotalSize-totalSize {
			return fmt.Errorf("capsule: v4 workspace total size exceeded")
		}
		totalSize += int64(len(file.Data))

		isMain := strings.HasPrefix(file.Path, mainRoot+"/")
		isLocal := strings.HasPrefix(file.Path, localRoot+"/")

		if !isMain && !isLocal {
			return fmt.Errorf(
				"capsule: file outside v4 module roots: %s", file.Path,
			)
		}

		metadata := FileMetadata{
			Path:   file.Path,
			Size:   int64(len(file.Data)),
			SHA256: SHA256(file.Data),
		}
		workspaceMetadata = append(workspaceMetadata, metadata)

		if isMain && file.Path == path.Join(mainRoot, "go.mod") {
			foundMainMod = true
			mainGoMod = file.Data
		}
		if isLocal {
			relative := strings.TrimPrefix(file.Path, localRoot+"/")
			if relative == "go.mod" {
				foundLocalMod = true
				localGoMod = file.Data
			}
			localMetadata = append(localMetadata, FileMetadata{
				Path:   relative,
				Size:   metadata.Size,
				SHA256: metadata.SHA256,
			})
		}
	}

	if !foundMainMod || !foundLocalMod {
		return fmt.Errorf("capsule: missing v4 module go.mod")
	}

	// Check actual captured module declarations against the selected
	// graph before publishing any archive bytes.
	if err := ValidateGoModuleConsistencyV4(
		mainGoMod,
		manifest.GoSelectedModules,
	); err != nil {
		return fmt.Errorf(
			"capsule: inconsistent v4 selected module graph: %w",
			err,
		)
	}

	localParsed, err := modfile.Parse(
		path.Join(localRoot, "go.mod"),
		localGoMod,
		nil,
	)
	if err != nil {
		return fmt.Errorf("capsule: parse v4 local go.mod: %w", err)
	}

	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path != manifest.LocalModules[0].ModulePath {
		return fmt.Errorf(
			"capsule: v4 local module identity does not match manifest",
		)
	}

	// Detect file/directory conflicts, including a file used as
	// an ancestor of another workspace file.
	for name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf(
					"capsule: v4 file/directory conflict: %s", parent,
				)
			}
		}
	}

	manifest.Workspace.Files = workspaceMetadata
	manifest.LocalModules[0].Files = localMetadata

	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("capsule: invalid v4 writer manifest: %w", err)
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("capsule: encode v4 manifest: %w", err)
	}
	if len(manifestData) > MaxManifestSize {
		return fmt.Errorf("capsule: v4 manifest exceeds size limit")
	}

	writeArchive := func(dst io.Writer) error {
		tw := tar.NewWriter(dst)

		writeEntry := func(name string, data []byte, mode int64) error {
			header := &tar.Header{
				Name:     name,
				Mode:     mode,
				Size:     int64(len(data)),
				Typeflag: tar.TypeReg,
			}
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf(
					"capsule: v4 TAR header %q: %w", name, err,
				)
			}
			if _, err := tw.Write(data); err != nil {
				return fmt.Errorf(
					"capsule: v4 TAR data %q: %w", name, err,
				)
			}
			return nil
		}

		if err := writeEntry("manifest.json", manifestData, 0600); err != nil {
			return err
		}

		for _, file := range ordered {
			if err := writeEntry(
				"workspace/"+file.Path,
				file.Data,
				int64(file.Mode.Perm()),
			); err != nil {
				return err
			}
		}

		if err := tw.Close(); err != nil {
			return fmt.Errorf("capsule: close v4 TAR: %w", err)
		}
		return nil
	}

	// Check the complete TAR serialization and size without
	// writing any data to the caller's destination.
	preflight := &workspaceSnapshotLimitWriterV3{
		dst:       io.Discard,
		remaining: MaxCapsuleSizeV3,
	}
	if err := writeArchive(preflight); err != nil {
		return fmt.Errorf("capsule: v4 archive preflight: %w", err)
	}

	limited := &workspaceSnapshotLimitWriterV3{
		dst:       w,
		remaining: MaxCapsuleSizeV3,
	}
	if err := writeArchive(limited); err != nil {
		return fmt.Errorf("capsule: write v4 archive: %w", err)
	}
	return nil
}
