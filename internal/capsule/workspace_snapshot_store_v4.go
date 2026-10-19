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

// WriteWorkspaceSnapshotFromStoreV4 creates the workspace TAR
// for the initial schema-v4 mixed Go dependency graph.
//
// External dependency artifacts belong exclusively in the
// separate module-cache TAR.
//
// Input validation completes before destination output begins.
// On an output error, callers must discard the partial TAR.
func WriteWorkspaceSnapshotFromStoreV4(
	w io.Writer,
	verified VerifiedStoredCapsuleV4,
) error {
	if w == nil {
		return fmt.Errorf("capsule: nil mixed v4 workspace writer")
	}
	if verified.Store == nil {
		return fmt.Errorf("capsule: missing mixed v4 artifact store")
	}

	manifest := verified.Manifest

	if manifest.SchemaVersion != SchemaVersionV4 {
		return fmt.Errorf("capsule: mixed workspace requires schema v4")
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf(
			"capsule: invalid mixed workspace manifest: %w", err,
		)
	}

	if len(manifest.LocalModules) != 1 {
		return fmt.Errorf(
			"capsule: mixed workspace requires one local module",
		)
	}
	if len(manifest.GoGraphModFiles) != 0 ||
		len(manifest.GoGraphInfoFiles) != 0 {
		return fmt.Errorf(
			"capsule: unsupported mixed workspace graph artifacts",
		)
	}
	if err := ValidateMixedGoModuleSetV4(
		manifest.GoSelectedModules, manifest.GoModules,
	); err != nil {
		return err
	}
	if err := ValidateGoModulesV3(manifest.GoModules); err != nil {
		return err
	}

	if len(verified.Files) != len(manifest.Workspace.Files) {
		return fmt.Errorf(
			"capsule: mixed workspace file count mismatch",
		)
	}

	expected := make(
		map[string]FileMetadata,
		len(manifest.Workspace.Files),
	)
	for _, metadata := range manifest.Workspace.Files {
		if _, exists := expected[metadata.Path]; exists {
			return fmt.Errorf(
				"capsule: duplicate mixed workspace metadata %q",
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
	var mainGoMod, localGoMod, goSum []byte
	var foundMain, foundLocal, foundSum bool

	var totalSize int64
	var estimated int64 = 1024

	for _, file := range files {
		if err := ValidateWorkspacePath(
			file.Path, file.Mode,
		); err != nil {
			return fmt.Errorf(
				"capsule: invalid mixed workspace path: %w", err,
			)
		}

		if seen[file.Path] {
			return fmt.Errorf(
				"capsule: duplicate mixed workspace file %q",
				file.Path,
			)
		}
		seen[file.Path] = true

		if !strings.HasPrefix(file.Path, mainRoot+"/") &&
			!strings.HasPrefix(file.Path, localRoot+"/") {
			return fmt.Errorf(
				"capsule: mixed workspace file outside roots: %q",
				file.Path,
			)
		}

		metadata, ok := expected[file.Path]
		if !ok {
			return fmt.Errorf(
				"capsule: unexpected mixed workspace file %q",
				file.Path,
			)
		}
		if err := VerifyWorkspaceFileV2(
			metadata, file.Data,
		); err != nil {
			return fmt.Errorf(
				"capsule: invalid mixed workspace file %q: %w",
				file.Path, err,
			)
		}

		size := int64(len(file.Data))
		if size > MaxWorkspaceFileSize ||
			size > MaxWorkspaceV2TotalSize-totalSize {
			return fmt.Errorf(
				"capsule: mixed workspace size exceeded",
			)
		}
		totalSize += size

		if estimated > MaxCapsuleSizeV3 ||
			tarEntrySize(size) >
				MaxCapsuleSizeV3-estimated {
			return fmt.Errorf(
				"capsule: mixed workspace TAR size exceeded",
			)
		}
		estimated += tarEntrySize(size)

		switch file.Path {
		case path.Join(mainRoot, "go.mod"):
			mainGoMod = file.Data
			foundMain = true
		case path.Join(mainRoot, "go.sum"):
			goSum = file.Data
			foundSum = true
		case path.Join(localRoot, "go.mod"):
			localGoMod = file.Data
			foundLocal = true
		}
	}

	if !foundMain || !foundLocal || !foundSum {
		return fmt.Errorf(
			"capsule: missing mixed workspace Go metadata",
		)
	}

	for name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf(
					"capsule: mixed workspace file/directory conflict %q",
					parent,
				)
			}
		}
	}

	if err := ValidateGoModuleConsistencyV4(
		mainGoMod, manifest.GoSelectedModules,
	); err != nil {
		return fmt.Errorf(
			"capsule: inconsistent mixed workspace Go graph: %w",
			err,
		)
	}

	localParsed, err := modfile.Parse(
		path.Join(localRoot, "go.mod"),
		localGoMod, nil,
	)
	if err != nil {
		return fmt.Errorf(
			"capsule: parse mixed local go.mod: %w", err,
		)
	}
	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path !=
			manifest.LocalModules[0].ModulePath {
		return fmt.Errorf(
			"capsule: mixed workspace local module identity mismatch",
		)
	}

	// Independently validate the external module artifact set.
	// Module bytes are not copied into this workspace TAR.
	if err := VerifyMixedGoModuleArtifactsV4(
		manifest.GoSelectedModules,
		manifest.GoModules,
		goSum,
		verified.Store,
	); err != nil {
		return fmt.Errorf(
			"capsule: verify mixed workspace dependencies: %w",
			err,
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
					"capsule: mixed workspace header %q: %w",
					file.Path, err,
				)
			}
			if _, err := tw.Write(file.Data); err != nil {
				return fmt.Errorf(
					"capsule: mixed workspace data %q: %w",
					file.Path, err,
				)
			}
		}

		if err := tw.Close(); err != nil {
			return fmt.Errorf(
				"capsule: close mixed workspace TAR: %w",
				err,
			)
		}
		return nil
	}

	// Validate TAR serialization without publishing any bytes.
	preflight := &workspaceSnapshotLimitWriterV3{
		dst:       io.Discard,
		remaining: MaxCapsuleSizeV3,
	}
	if err := writeTAR(preflight); err != nil {
		return fmt.Errorf(
			"capsule: mixed workspace TAR preflight: %w", err,
		)
	}

	limited := &workspaceSnapshotLimitWriterV3{
		dst:       w,
		remaining: MaxCapsuleSizeV3,
	}
	if err := writeTAR(limited); err != nil {
		return fmt.Errorf(
			"capsule: write mixed workspace TAR: %w", err,
		)
	}

	return nil
}
