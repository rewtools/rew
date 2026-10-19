package capsule

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"syscall"

	"golang.org/x/mod/modfile"
)

// VerifiedStoredCapsuleV4 owns verified workspace bytes and a
// private disk-backed external module artifact store.
//
// The caller must call Close when finished.
type VerifiedStoredCapsuleV4 struct {
	Manifest ManifestV4
	Files    []WorkspaceFile
	Store    *ArtifactStoreV3
}

func (v *VerifiedStoredCapsuleV4) Close() error {
	if v == nil || v.Store == nil {
		return nil
	}
	err := v.Store.Close()
	v.Store = nil
	return err
}

// ReadVerifiedToStoreV4 verifies the initial mixed-dependency
// schema-v4 archive containing exactly one local replacement and
// one external module.
//
// It does not support graph-only artifacts yet, and does not
// establish capture completeness or safe execution.
func ReadVerifiedToStoreV4(
	filename string,
) (result VerifiedStoredCapsuleV4, resultErr error) {
	f, err := os.OpenFile(
		filename,
		os.O_RDONLY|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return result, fmt.Errorf("capsule: open mixed v4 archive: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() ||
		info.Size() < 1024 ||
		info.Size() > MaxCapsuleSizeV3 {
		return result, fmt.Errorf("capsule: invalid mixed v4 archive size or type")
	}

	tr := tar.NewReader(io.LimitReader(f, MaxCapsuleSizeV3+1))

	header, err := tr.Next()
	if err != nil {
		return result, fmt.Errorf("capsule: read mixed v4 manifest: %w", err)
	}
	if header.Name != "manifest.json" ||
		header.Typeflag != tar.TypeReg ||
		header.Size < 0 ||
		header.Size > MaxManifestSize {
		return result, fmt.Errorf("capsule: invalid mixed v4 manifest entry")
	}

	manifestData, err := io.ReadAll(
		io.LimitReader(tr, MaxManifestSize+1),
	)
	if err != nil {
		return result, err
	}
	if int64(len(manifestData)) != header.Size {
		return result, fmt.Errorf("capsule: mixed v4 manifest size mismatch")
	}

	if err := json.Unmarshal(manifestData, &result.Manifest); err != nil {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: decode mixed v4 manifest: %w", err,
		)
	}

	m := result.Manifest

	if err := m.Validate(); err != nil {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: invalid mixed v4 manifest: %w", err,
		)
	}

	if len(m.GoGraphModFiles) != 0 ||
		len(m.GoGraphInfoFiles) != 0 {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: mixed v4 graph-only artifacts unsupported",
		)
	}

	if err := ValidateMixedGoModuleSetV4(
		m.GoSelectedModules, m.GoModules,
	); err != nil {
		return VerifiedStoredCapsuleV4{}, err
	}

	if err := ValidateGoModulesV3(m.GoModules); err != nil {
		return VerifiedStoredCapsuleV4{}, err
	}

	if len(m.Workspace.Files) == 0 ||
		len(m.Workspace.Files) > MaxWorkspaceV3Files {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: invalid mixed v4 workspace file count",
		)
	}

	var workspaceTotal int64
	previousPath := ""

	for _, metadata := range m.Workspace.Files {
		if metadata.Path <= previousPath {
			return VerifiedStoredCapsuleV4{}, fmt.Errorf(
				"capsule: unordered mixed v4 workspace",
			)
		}
		previousPath = metadata.Path

		if metadata.Size < 0 ||
			metadata.Size > MaxWorkspaceFileSize ||
			metadata.Size > MaxWorkspaceV2TotalSize-workspaceTotal {
			return VerifiedStoredCapsuleV4{}, fmt.Errorf(
				"capsule: mixed v4 workspace size limit exceeded",
			)
		}
		workspaceTotal += metadata.Size
	}

	expected := expectedTarSize(
		int64(len(manifestData)), m.Workspace,
	)
	if expected < 0 || expected > MaxCapsuleSizeV3 {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: mixed v4 archive size exceeded",
		)
	}

	for _, mod := range m.GoModules {
		for _, artifact := range mod.Artifacts {
			if artifact.Size < 0 ||
				artifact.Size > MaxCapsuleSizeV3-expected {
				return VerifiedStoredCapsuleV4{}, fmt.Errorf(
					"capsule: mixed v4 dependency size exceeded",
				)
			}
			entrySize := tarEntrySize(artifact.Size)
			if entrySize > MaxCapsuleSizeV3-expected {
				return VerifiedStoredCapsuleV4{}, fmt.Errorf(
					"capsule: mixed v4 TAR budget exceeded",
				)
			}
			expected += entrySize
		}
	}

	if info.Size() != expected {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: mixed v4 archive size mismatch: expected %d, got %d",
			expected, info.Size(),
		)
	}

	result.Files = make(
		[]WorkspaceFile, 0, len(m.Workspace.Files),
	)

	mainModPath := path.Join(m.Execution.WorkingDir, "go.mod")
	mainSumPath := path.Join(m.Execution.WorkingDir, "go.sum")
	localModPath := path.Join(m.LocalModules[0].SnapshotPath, "go.mod")

	var mainGoMod, localGoMod, goSum []byte
	foundMain, foundLocal, foundSum := false, false, false

	for _, metadata := range m.Workspace.Files {
		entry, err := tr.Next()
		if err != nil {
			return VerifiedStoredCapsuleV4{}, fmt.Errorf(
				"capsule: missing mixed v4 workspace %q: %w",
				metadata.Path, err,
			)
		}

		if entry.Name != "workspace/"+metadata.Path ||
			entry.Typeflag != tar.TypeReg ||
			entry.Size != metadata.Size {
			return VerifiedStoredCapsuleV4{}, fmt.Errorf(
				"capsule: invalid mixed v4 workspace TAR entry %q",
				metadata.Path,
			)
		}

		mode := os.FileMode(entry.Mode)
		if err := ValidateWorkspacePath(metadata.Path, mode); err != nil {
			return VerifiedStoredCapsuleV4{}, err
		}

		data, err := io.ReadAll(
			io.LimitReader(tr, MaxWorkspaceFileSize+1),
		)
		if err != nil {
			return VerifiedStoredCapsuleV4{}, err
		}

		if err := VerifyWorkspaceFileV2(metadata, data); err != nil {
			return VerifiedStoredCapsuleV4{}, fmt.Errorf(
				"capsule: verify mixed v4 workspace %q: %w",
				metadata.Path, err,
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
		case mainSumPath:
			goSum = data
			foundSum = true
		}
	}

	if !foundMain || !foundLocal || !foundSum {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: mixed v4 archive missing required go.mod or go.sum",
		)
	}

	if err := ValidateGoModuleConsistencyV4(
		mainGoMod, m.GoSelectedModules,
	); err != nil {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: invalid mixed v4 dependency graph: %w", err,
		)
	}

	localParsed, err := modfile.Parse(localModPath, localGoMod, nil)
	if err != nil {
		return VerifiedStoredCapsuleV4{}, err
	}
	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path != m.LocalModules[0].ModulePath {
		return VerifiedStoredCapsuleV4{}, fmt.Errorf(
			"capsule: mixed v4 local module identity mismatch",
		)
	}

	store, err := NewArtifactStoreV3(MaxGoModuleTotalSizeV3)
	if err != nil {
		return VerifiedStoredCapsuleV4{}, err
	}
	result.Store = store

	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, store.Close())
			result = VerifiedStoredCapsuleV4{}
		}
	}()

	for _, mod := range m.GoModules {
		for _, artifact := range mod.Artifacts {
			entry, err := tr.Next()
			if err != nil {
				return result, fmt.Errorf(
					"capsule: missing mixed v4 artifact %q: %w",
					artifact.Path, err,
				)
			}

			if entry.Name != "dependencies/"+artifact.Path ||
				entry.Typeflag != tar.TypeReg ||
				entry.Size != artifact.Size {
				return result, fmt.Errorf(
					"capsule: invalid mixed v4 dependency TAR entry %q",
					artifact.Path,
				)
			}

			if !strings.HasPrefix(artifact.SHA256, "sha256:") {
				return result, fmt.Errorf(
					"capsule: invalid mixed v4 artifact digest",
				)
			}

			if err := store.Put(
				artifact.Path,
				tr,
				artifact.Size,
				strings.TrimPrefix(artifact.SHA256, "sha256:"),
			); err != nil {
				return result, fmt.Errorf(
					"capsule: verify stored mixed v4 artifact %q: %w",
					artifact.Path, err,
				)
			}
		}
	}

	if _, err := tr.Next(); err != io.EOF {
		return result, fmt.Errorf(
			"capsule: unexpected extra mixed v4 TAR entry or error: %v",
			err,
		)
	}

	if err := VerifyMixedGoModuleArtifactsV4(
		m.GoSelectedModules,
		m.GoModules,
		goSum,
		store,
	); err != nil {
		return result, fmt.Errorf(
			"capsule: verify mixed v4 external modules: %w", err,
		)
	}

	return result, nil
}
