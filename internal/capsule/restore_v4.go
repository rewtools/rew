package capsule

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// RestoreWorkspaceV4 restores a verified workspace-only V4 capsule
// into an existing, empty, non-symlink destination directory.
//
// The caller must keep the destination private from concurrent
// modification. This function does not provide atomic publication.
func RestoreWorkspaceV4(
	destination string,
	verified VerifiedCapsuleV4,
) error {
	manifest := verified.Manifest

	if manifest.SchemaVersion != SchemaVersionV4 {
		return fmt.Errorf("capsule: restore requires schema v4")
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("capsule: invalid v4 restore manifest: %w", err)
	}

	if len(manifest.GoModules) != 0 ||
		len(manifest.GoGraphInfoFiles) != 0 ||
		len(manifest.GoGraphModFiles) != 0 {
		return fmt.Errorf(
			"capsule: v4 restore does not support external artifacts",
		)
	}

	if len(verified.Files) != len(manifest.Workspace.Files) {
		return fmt.Errorf("capsule: v4 restore file count mismatch")
	}

	// Validate every file before creating any output.
	seen := make(map[string]bool, len(verified.Files))
	var totalSize int64

	for i, file := range verified.Files {
		metadata := manifest.Workspace.Files[i]

		if file.Path != metadata.Path {
			return fmt.Errorf(
				"capsule: v4 restore file order mismatch: %q",
				file.Path,
			)
		}

		if err := ValidateWorkspacePath(file.Path, file.Mode); err != nil {
			return fmt.Errorf(
				"capsule: invalid v4 restore path: %w",
				err,
			)
		}

		if seen[file.Path] {
			return fmt.Errorf(
				"capsule: duplicate v4 restore path: %q",
				file.Path,
			)
		}
		seen[file.Path] = true

		if int64(len(file.Data)) > MaxWorkspaceFileSize ||
			int64(len(file.Data)) > MaxWorkspaceV2TotalSize-totalSize {
			return fmt.Errorf("capsule: v4 restore size limit exceeded")
		}
		totalSize += int64(len(file.Data))

		if err := VerifyWorkspaceFileV2(metadata, file.Data); err != nil {
			return fmt.Errorf(
				"capsule: invalid v4 restore file %q: %w",
				file.Path,
				err,
			)
		}
	}

	for name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf(
					"capsule: v4 restore file/directory conflict: %q",
					parent,
				)
			}
		}
	}

	// Reject symlink destinations before opening the destination root.
	info, err := os.Lstat(destination)
	if err != nil {
		return fmt.Errorf("capsule: inspect v4 restore destination: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("capsule: invalid v4 restore destination")
	}

	root, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("capsule: open v4 restore root: %w", err)
	}
	defer root.Close()

	// This implementation deliberately requires a new, empty staging
	// directory rather than merging files into an existing tree.
	dir, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("capsule: open v4 restore directory: %w", err)
	}

	entries, readErr := dir.ReadDir(1)
	closeErr := dir.Close()

	if readErr != nil && readErr != io.EOF {
		return fmt.Errorf("capsule: inspect v4 restore root: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("capsule: close v4 restore directory: %w", closeErr)
	}
	if len(entries) != 0 {
		return fmt.Errorf("capsule: v4 restore destination is not empty")
	}

	for _, file := range verified.Files {
		parent := path.Dir(file.Path)

		if parent != "." {
			if err := root.MkdirAll(parent, 0700); err != nil {
				return fmt.Errorf(
					"capsule: create v4 restore directory %q: %w",
					parent,
					err,
				)
			}
		}

		mode := file.Mode.Perm()
		if mode&fs.ModePerm == 0 {
			mode = 0600
		}

		out, err := root.OpenFile(
			file.Path,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			mode,
		)
		if err != nil {
			return fmt.Errorf(
				"capsule: create v4 restore file %q: %w",
				file.Path,
				err,
			)
		}

		n, writeErr := out.Write(file.Data)
		closeErr := out.Close()

		if writeErr != nil {
			return fmt.Errorf(
				"capsule: write v4 restore file %q: %w",
				file.Path,
				writeErr,
			)
		}
		if n != len(file.Data) {
			return fmt.Errorf(
				"capsule: short v4 restore write: %q",
				file.Path,
			)
		}
		if closeErr != nil {
			return fmt.Errorf(
				"capsule: close v4 restore file %q: %w",
				file.Path,
				closeErr,
			)
		}
	}

	// Avoid silently accepting an unexpected virtual workspace layout.
	mainPrefix := manifest.Execution.WorkingDir + "/"
	localPrefix := manifest.LocalModules[0].SnapshotPath + "/"

	for _, file := range verified.Files {
		if !strings.HasPrefix(file.Path, mainPrefix) &&
			!strings.HasPrefix(file.Path, localPrefix) {
			return fmt.Errorf(
				"capsule: restored path outside v4 module roots: %q",
				file.Path,
			)
		}
	}

	return nil
}
