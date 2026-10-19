package capsule

import (
	"fmt"
	"path"
	"strings"
)

// MapLocalWorkspaceV4 maps already-read files from the main and
// replacement Go modules into a single virtual workspace.
//
// It does not access the filesystem or establish the safety of
// physical module roots. Those checks belong to the capture reader.
func MapLocalWorkspaceV4(
	mainRoot string,
	mainFiles []WorkspaceFile,
	localRoot string,
	localFiles []WorkspaceFile,
) ([]WorkspaceFile, error) {
	if err := validateV4SnapshotDirectory(mainRoot); err != nil {
		return nil, fmt.Errorf("capsule: invalid main snapshot root: %w", err)
	}

	if err := validateV4SnapshotDirectory(localRoot); err != nil {
		return nil, fmt.Errorf("capsule: invalid local snapshot root: %w", err)
	}

	if mainRoot == localRoot ||
		strings.HasPrefix(mainRoot, localRoot+"/") ||
		strings.HasPrefix(localRoot, mainRoot+"/") {
		return nil, fmt.Errorf("capsule: overlapping v4 snapshot roots")
	}

	totalCount := len(mainFiles) + len(localFiles)
	if totalCount > MaxWorkspaceV3Files {
		return nil, fmt.Errorf("capsule: too many v4 workspace files")
	}

	result := make([]WorkspaceFile, 0, totalCount)
	seen := make(map[string]bool, totalCount)
	parentDirs := make(map[string]bool)
	var totalSize int64

	addModule := func(
		root string,
		files []WorkspaceFile,
	) error {
		foundGoMod := false

		for _, file := range files {
			if err := ValidateWorkspacePath(file.Path, file.Mode); err != nil {
				return fmt.Errorf(
					"capsule: invalid v4 module file: %w",
					err,
				)
			}

			if file.Path == "go.mod" {
				foundGoMod = true
			}

			if int64(len(file.Data)) > MaxWorkspaceFileSize {
				return fmt.Errorf(
					"capsule: v4 module file too large: %s",
					file.Path,
				)
			}

			if int64(len(file.Data)) >
				MaxWorkspaceV2TotalSize-totalSize {
				return fmt.Errorf("capsule: v4 workspace size exceeded")
			}

			virtualPath := path.Join(root, file.Path)

			if seen[virtualPath] {
				return fmt.Errorf(
					"capsule: duplicate v4 workspace path: %s",
					virtualPath,
				)
			}
			if parentDirs[virtualPath] {
				return fmt.Errorf(
					"capsule: file/directory conflict: %s",
					virtualPath,
				)
			}

			for parent := path.Dir(virtualPath); parent != "."; parent = path.Dir(parent) {
				if seen[parent] {
					return fmt.Errorf(
						"capsule: file/directory conflict: %s",
						parent,
					)
				}
				parentDirs[parent] = true
			}

			seen[virtualPath] = true

			result = append(result, WorkspaceFile{
				Path: virtualPath,
				Data: append([]byte(nil), file.Data...),
				Mode: file.Mode,
			})

			totalSize += int64(len(file.Data))
		}

		if !foundGoMod {
			return fmt.Errorf(
				"capsule: missing v4 module go.mod under %s",
				root,
			)
		}

		return nil
	}

	if err := addModule(mainRoot, mainFiles); err != nil {
		return nil, err
	}

	if err := addModule(localRoot, localFiles); err != nil {
		return nil, err
	}

	return result, nil
}
