package capsule

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// WriteGoModuleSnapshotFromStoreV4 creates a Go download-cache TAR
// from the initial schema-v4 mixed-dependency artifact set.
//
// The destination must be discarded on any error. All module
// artifacts are checked before the first TAR byte is emitted.
// Streamed bytes are hashed again to detect mutation during copy.
func WriteGoModuleSnapshotFromStoreV4(
	w io.Writer,
	verified VerifiedStoredCapsuleV4,
) error {
	if w == nil {
		return fmt.Errorf("capsule: nil mixed v4 module snapshot writer")
	}
	if verified.Store == nil {
		return fmt.Errorf("capsule: missing mixed v4 artifact store")
	}

	m := verified.Manifest
	if m.SchemaVersion != SchemaVersionV4 {
		return fmt.Errorf("capsule: module snapshot requires schema v4")
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("capsule: invalid mixed v4 manifest: %w", err)
	}
	if len(m.GoGraphModFiles) != 0 ||
		len(m.GoGraphInfoFiles) != 0 {
		return fmt.Errorf("capsule: mixed v4 graph-only artifacts unsupported")
	}
	if err := ValidateMixedGoModuleSetV4(
		m.GoSelectedModules, m.GoModules,
	); err != nil {
		return err
	}
	if err := ValidateGoModulesV3(m.GoModules); err != nil {
		return err
	}
	if len(verified.Files) != len(m.Workspace.Files) {
		return fmt.Errorf("capsule: mixed v4 workspace count mismatch")
	}

	expected := make(map[string]FileMetadata, len(m.Workspace.Files))
	for _, metadata := range m.Workspace.Files {
		if _, exists := expected[metadata.Path]; exists {
			return fmt.Errorf("capsule: duplicate workspace metadata %q", metadata.Path)
		}
		expected[metadata.Path] = metadata
	}

	seen := make(map[string]bool, len(verified.Files))
	var mainGoMod, localGoMod, goSum []byte
	mainModPath := path.Join(m.Execution.WorkingDir, "go.mod")
	mainSumPath := path.Join(m.Execution.WorkingDir, "go.sum")
	localModPath := path.Join(m.LocalModules[0].SnapshotPath, "go.mod")

	for _, file := range verified.Files {
		if seen[file.Path] {
			return fmt.Errorf("capsule: duplicate workspace file %q", file.Path)
		}
		seen[file.Path] = true

		metadata, ok := expected[file.Path]
		if !ok {
			return fmt.Errorf("capsule: unexpected workspace file %q", file.Path)
		}
		if err := ValidateWorkspacePath(file.Path, file.Mode); err != nil {
			return err
		}
		if err := VerifyWorkspaceFileV2(metadata, file.Data); err != nil {
			return fmt.Errorf("capsule: verify workspace %q: %w", file.Path, err)
		}

		switch file.Path {
		case mainModPath:
			mainGoMod = file.Data
		case mainSumPath:
			goSum = file.Data
		case localModPath:
			localGoMod = file.Data
		}
	}

	if !seen[mainModPath] || !seen[mainSumPath] ||
		!seen[localModPath] {
		return fmt.Errorf("capsule: missing mixed v4 Go workspace metadata")
	}
	if err := ValidateGoModuleConsistencyV4(
		mainGoMod, m.GoSelectedModules,
	); err != nil {
		return fmt.Errorf("capsule: inconsistent mixed v4 Go graph: %w", err)
	}

	// The reader has already validated local module identity.
	// The snapshot writer independently verifies external artifacts.
	if len(localGoMod) == 0 {
		return fmt.Errorf("capsule: empty local go.mod")
	}

	localPath := path.Join(
		m.LocalModules[0].SnapshotPath, "go.mod",
	)

	localParsed, err := modfile.Parse(
		localPath, localGoMod, nil,
	)
	if err != nil {
		return fmt.Errorf(
			"capsule: parse mixed v4 local go.mod: %w",
			err,
		)
	}

	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path !=
			m.LocalModules[0].ModulePath {
		return fmt.Errorf(
			"capsule: mixed v4 local module identity mismatch",
		)
	}

	if err := VerifyMixedGoModuleArtifactsV4(
		m.GoSelectedModules, m.GoModules, goSum, verified.Store,
	); err != nil {
		return fmt.Errorf("capsule: verify mixed v4 module artifacts: %w", err)
	}

	type snapshotEntry struct {
		source      string
		destination string
		size        int64
		digest      string
	}
	var entries []snapshotEntry
	var estimated int64 = 1024

	for index, mod := range m.GoModules {
		escapedPath, err := module.EscapePath(mod.Path)
		if err != nil {
			return err
		}
		escapedVersion, err := module.EscapeVersion(mod.Version)
		if err != nil {
			return err
		}

		byName := make(map[string]GoModuleArtifact, len(mod.Artifacts))
		for _, artifact := range mod.Artifacts {
			byName[artifact.Path] = artifact
		}

		for _, ext := range []string{".zip", ".mod", ".info"} {
			source := fmt.Sprintf("module-%04d%s", index+1, ext)
			artifact, ok := byName[source]
			if !ok || !strings.HasPrefix(artifact.SHA256, "sha256:") {
				return fmt.Errorf("capsule: invalid module artifact %q", source)
			}
			if artifact.Size < 0 ||
				artifact.Size > MaxCapsuleSizeV3-estimated {
				return fmt.Errorf("capsule: module snapshot size exceeded")
			}
			entrySize := tarEntrySize(artifact.Size)
			if entrySize > MaxCapsuleSizeV3-estimated {
				return fmt.Errorf("capsule: module snapshot TAR budget exceeded")
			}
			estimated += entrySize

			entries = append(entries, snapshotEntry{
				source: source,
				destination: path.Join(
					"cache", "download", escapedPath,
					"@v", escapedVersion+ext,
				),
				size:   artifact.Size,
				digest: strings.TrimPrefix(artifact.SHA256, "sha256:"),
			})
		}
	}

	limited := &workspaceSnapshotLimitWriterV3{
		dst: w, remaining: MaxCapsuleSizeV3,
	}
	tw := tar.NewWriter(limited)

	for _, entry := range entries {
		file, err := verified.Store.Open(entry.source)
		if err != nil {
			return fmt.Errorf("capsule: open snapshot artifact %q: %w", entry.source, err)
		}

		header := &tar.Header{
			Name: entry.destination, Mode: 0644,
			Size: entry.size, Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			return errors.Join(err, file.Close())
		}

		hash := sha256.New()
		written, copyErr := io.CopyN(
			io.MultiWriter(tw, hash), file, entry.size,
		)
		closeErr := file.Close()

		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if written != entry.size ||
			hex.EncodeToString(hash.Sum(nil)) != entry.digest {
			return fmt.Errorf(
				"capsule: module artifact %q changed during snapshot copy",
				entry.source,
			)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("capsule: close mixed v4 module TAR: %w", err)
	}
	return nil
}
