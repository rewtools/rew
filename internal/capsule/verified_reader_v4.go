package capsule

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"syscall"

	"golang.org/x/mod/modfile"
)

// VerifiedCapsuleV4 contains workspace bytes that have passed
// archive, manifest, file-integrity, and Go module checks.
//
// This does not establish capture completeness or safe execution.
type VerifiedCapsuleV4 struct {
	Manifest ManifestV4
	Files    []WorkspaceFile
}

// ReadVerifiedV4 reads the experimental workspace-only V4 capsule.
//
// External Go dependency artifacts are deliberately unsupported.
// No archived content is extracted or executed by this function.
func ReadVerifiedV4(filename string) (VerifiedCapsuleV4, error) {
	var empty VerifiedCapsuleV4

	f, err := os.OpenFile(
		filename,
		os.O_RDONLY|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return empty, fmt.Errorf("capsule: open v4 archive: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return empty, fmt.Errorf("capsule: stat v4 archive: %w", err)
	}
	if !info.Mode().IsRegular() ||
		info.Size() > MaxCapsuleSizeV3 ||
		info.Size() < 1024 {
		return empty, fmt.Errorf("capsule: invalid v4 archive file or size")
	}

	tr := tar.NewReader(io.LimitReader(f, MaxCapsuleSizeV3+1))

	header, err := tr.Next()
	if err != nil {
		return empty, fmt.Errorf("capsule: read v4 manifest header: %w", err)
	}
	if header.Name != "manifest.json" ||
		header.Typeflag != tar.TypeReg ||
		header.Size < 0 ||
		header.Size > MaxManifestSize {
		return empty, fmt.Errorf("capsule: invalid v4 manifest entry")
	}

	manifestData, err := io.ReadAll(
		io.LimitReader(tr, MaxManifestSize+1),
	)
	if err != nil {
		return empty, fmt.Errorf("capsule: read v4 manifest: %w", err)
	}
	if int64(len(manifestData)) != header.Size {
		return empty, fmt.Errorf("capsule: v4 manifest size mismatch")
	}

	var result VerifiedCapsuleV4
	if err := json.Unmarshal(manifestData, &result.Manifest); err != nil {
		return empty, fmt.Errorf("capsule: decode v4 manifest: %w", err)
	}

	if result.Manifest.SchemaVersion != SchemaVersionV4 {
		return empty, fmt.Errorf("capsule: expected schema version 4")
	}
	if err := result.Manifest.Validate(); err != nil {
		return empty, fmt.Errorf("capsule: invalid v4 manifest: %w", err)
	}

	// This first reader supports precisely the artifact-free writer.
	if len(result.Manifest.GoModules) != 0 ||
		len(result.Manifest.GoGraphInfoFiles) != 0 ||
		len(result.Manifest.GoGraphModFiles) != 0 {
		return empty, fmt.Errorf(
			"capsule: v4 reader does not support external artifacts",
		)
	}

	expectedSize := expectedTarSize(
		int64(len(manifestData)),
		result.Manifest.Workspace,
	)
	if info.Size() != expectedSize {
		return empty, fmt.Errorf(
			"capsule: v4 archive size mismatch: expected %d, got %d",
			expectedSize,
			info.Size(),
		)
	}

	result.Files = make(
		[]WorkspaceFile,
		0,
		len(result.Manifest.Workspace.Files),
	)

	mainModPath := path.Join(result.Manifest.Execution.WorkingDir, "go.mod")
	localModPath := path.Join(
		result.Manifest.LocalModules[0].SnapshotPath,
		"go.mod",
	)

	var mainGoMod, localGoMod []byte
	foundMain, foundLocal := false, false

	for _, metadata := range result.Manifest.Workspace.Files {
		header, err := tr.Next()
		if err != nil {
			return empty, fmt.Errorf(
				"capsule: read v4 workspace entry %q: %w",
				metadata.Path,
				err,
			)
		}

		if header.Name != "workspace/"+metadata.Path ||
			header.Typeflag != tar.TypeReg ||
			header.Size != metadata.Size {
			return empty, fmt.Errorf(
				"capsule: invalid v4 workspace entry %q",
				metadata.Path,
			)
		}

		mode := os.FileMode(header.Mode)
		if err := ValidateWorkspacePath(metadata.Path, mode); err != nil {
			return empty, fmt.Errorf(
				"capsule: invalid v4 workspace path %q: %w",
				metadata.Path,
				err,
			)
		}

		data, err := io.ReadAll(
			io.LimitReader(tr, MaxWorkspaceFileSize+1),
		)
		if err != nil {
			return empty, fmt.Errorf(
				"capsule: read v4 workspace data %q: %w",
				metadata.Path,
				err,
			)
		}

		if err := VerifyWorkspaceFileV2(metadata, data); err != nil {
			return empty, fmt.Errorf(
				"capsule: verify v4 workspace file %q: %w",
				metadata.Path,
				err,
			)
		}

		result.Files = append(result.Files, WorkspaceFile{
			Path: metadata.Path,
			Data: data,
			Mode: mode,
		})

		switch metadata.Path {
		case mainModPath:
			mainGoMod = data
			foundMain = true
		case localModPath:
			localGoMod = data
			foundLocal = true
		}
	}

	if !foundMain || !foundLocal {
		return empty, fmt.Errorf("capsule: missing v4 module go.mod")
	}

	if err := ValidateGoModuleConsistencyV4(
		mainGoMod,
		result.Manifest.GoSelectedModules,
	); err != nil {
		return empty, fmt.Errorf(
			"capsule: invalid v4 selected module graph: %w",
			err,
		)
	}

	localParsed, err := modfile.Parse(
		localModPath,
		localGoMod,
		nil,
	)
	if err != nil {
		return empty, fmt.Errorf(
			"capsule: parse local v4 go.mod: %w",
			err,
		)
	}

	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path !=
			result.Manifest.LocalModules[0].ModulePath {
		return empty, fmt.Errorf(
			"capsule: v4 local module identity mismatch",
		)
	}

	if _, err := tr.Next(); err != io.EOF {
		if err == nil {
			return empty, fmt.Errorf("capsule: unexpected extra v4 TAR entry")
		}
		return empty, fmt.Errorf(
			"capsule: invalid v4 TAR termination: %w",
			err,
		)
	}

	return result, nil
}
