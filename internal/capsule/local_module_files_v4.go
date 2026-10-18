package capsule

import (
	"fmt"
	"path"
	"strings"
)

// validateLocalModuleFilesV4 validates file metadata for the
// currently supported single-local-replacement layout.
//
// It checks manifest metadata only. Actual file bytes must be
// verified independently by a future schema-v4 reader.
func validateLocalModuleFilesV4(m ManifestV4) error {
	if err := validateWorkspaceV3(m.Workspace); err != nil {
		return fmt.Errorf("capsule: invalid v4 workspace metadata: %w", err)
	}

	mainGoMod := path.Join(m.Execution.WorkingDir, "go.mod")
	workspaceFiles := make(map[string]FileMetadata, len(m.Workspace.Files))

	for _, file := range m.Workspace.Files {
		workspaceFiles[file.Path] = file
	}

	if _, ok := workspaceFiles[mainGoMod]; !ok {
		return fmt.Errorf("capsule: missing v4 main module go.mod")
	}

	if len(m.LocalModules) != 1 {
		return fmt.Errorf("capsule: expected one local module")
	}

	local := m.LocalModules[0]

	if len(local.Files) == 0 {
		return fmt.Errorf("capsule: missing local module file metadata")
	}

	if len(local.Files) > MaxWorkspaceV3Files {
		return fmt.Errorf("capsule: too many local module files")
	}

	seen := make(map[string]bool, len(local.Files))
	foundGoMod := false

	for _, file := range local.Files {
		// Validate local paths using the same canonical-file
		// rules as workspace paths.
		if err := ValidateWorkspacePath(file.Path, 0600); err != nil {
			return fmt.Errorf("capsule: invalid local file path: %w", err)
		}

		if seen[file.Path] {
			return fmt.Errorf("capsule: duplicate local module file")
		}
		seen[file.Path] = true

		if file.Path == "go.mod" {
			foundGoMod = true
		}

		fullPath := path.Join(local.SnapshotPath, file.Path)

		actual, ok := workspaceFiles[fullPath]
		if !ok {
			return fmt.Errorf(
				"capsule: local module file missing from workspace: %s",
				fullPath,
			)
		}

		if actual.Size != file.Size || actual.SHA256 != file.SHA256 {
			return fmt.Errorf(
				"capsule: local module file metadata mismatch: %s",
				fullPath,
			)
		}
	}

	if !foundGoMod {
		return fmt.Errorf("capsule: missing local module go.mod")
	}

	// Every workspace file inside the replacement module must
	// have corresponding local-module metadata.
	prefix := local.SnapshotPath + "/"

	for workspacePath := range workspaceFiles {
		if workspacePath == local.SnapshotPath {
			return fmt.Errorf("capsule: local module root is a file")
		}

		if !strings.HasPrefix(workspacePath, prefix) {
			continue
		}

		relativePath := strings.TrimPrefix(workspacePath, prefix)

		if !seen[relativePath] {
			return fmt.Errorf(
				"capsule: unlisted local module workspace file: %s",
				workspacePath,
			)
		}
	}

	return nil
}
