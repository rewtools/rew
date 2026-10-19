package capsule

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// WriteWorkspaceFromStoreV4 emits the initial mixed-dependency V4
// archive using one local replacement and one external Go module.
//
// Workspace validation is delegated to the existing V4 writer.
// External dependencies are verified from a bounded disk-backed
// store before any bytes are written to the destination.
//
// A destination failure or concurrent store mutation can still
// leave a partial archive; callers must discard it on error.
func WriteWorkspaceFromStoreV4(
	w io.Writer,
	manifest ManifestV4,
	files []WorkspaceFile,
	store *ArtifactStoreV3,
) error {
	if w == nil {
		return fmt.Errorf("capsule: nil mixed v4 writer")
	}
	if len(manifest.GoGraphModFiles) != 0 ||
		len(manifest.GoGraphInfoFiles) != 0 {
		return fmt.Errorf("capsule: mixed v4 graph artifacts unsupported")
	}

	if err := ValidateMixedGoModuleSetV4(
		manifest.GoSelectedModules,
		manifest.GoModules,
	); err != nil {
		return err
	}

	if err := ValidateGoModulesV3(manifest.GoModules); err != nil {
		return err
	}

	var goSum []byte
	foundSum := false
	sumPath := path.Join(manifest.Execution.WorkingDir, "go.sum")

	for _, file := range files {
		if file.Path == sumPath {
			if foundSum {
				return fmt.Errorf("capsule: duplicate mixed v4 go.sum")
			}
			foundSum = true
			goSum = file.Data
		}
	}
	if !foundSum {
		return fmt.Errorf("capsule: missing mixed v4 go.sum")
	}

	if err := VerifyMixedGoModuleArtifactsV4(
		manifest.GoSelectedModules,
		manifest.GoModules,
		goSum,
		store,
	); err != nil {
		return err
	}

	// Use the existing V4 writer to derive and validate workspace
	// metadata, local module identities, and selected Go graph.
	base := manifest
	base.GoModules = nil

	tmp, err := os.CreateTemp("", "rew-v4-workspace-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if err := WriteWorkspaceV4(tmp, base, files); err != nil {
		return fmt.Errorf("capsule: validate mixed v4 workspace: %w", err)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	reader := tar.NewReader(tmp)
	header, err := reader.Next()
	if err != nil {
		return err
	}
	if header.Name != "manifest.json" ||
		header.Typeflag != tar.TypeReg ||
		header.Size < 0 ||
		header.Size > MaxManifestSize {
		return fmt.Errorf("capsule: invalid intermediate v4 manifest")
	}

	baseJSON, err := io.ReadAll(
		io.LimitReader(reader, MaxManifestSize+1),
	)
	if err != nil {
		return err
	}
	if int64(len(baseJSON)) != header.Size {
		return fmt.Errorf("capsule: incomplete intermediate v4 manifest")
	}

	var derived ManifestV4
	if err := json.Unmarshal(baseJSON, &derived); err != nil {
		return err
	}

	derived.GoModules = manifest.GoModules
	if err := derived.Validate(); err != nil {
		return fmt.Errorf("capsule: invalid mixed v4 manifest: %w", err)
	}

	manifestJSON, err := json.Marshal(derived)
	if err != nil {
		return err
	}
	if len(manifestJSON) > MaxManifestSize {
		return fmt.Errorf("capsule: mixed v4 manifest too large")
	}

	// Preflight the full canonical archive, including TAR padding.
	expected := expectedTarSize(
		int64(len(manifestJSON)),
		derived.Workspace,
	)
	if expected < 0 || expected > MaxCapsuleSizeV3 {
		return fmt.Errorf("capsule: mixed v4 archive too large")
	}
	for _, mod := range derived.GoModules {
		for _, artifact := range mod.Artifacts {
			if artifact.Size < 0 ||
				artifact.Size > MaxCapsuleSizeV3-expected {
				return fmt.Errorf("capsule: mixed v4 artifact too large")
			}
			entrySize := tarEntrySize(artifact.Size)
			if entrySize > MaxCapsuleSizeV3-expected {
				return fmt.Errorf("capsule: mixed v4 TAR size exceeded")
			}
			expected += entrySize
		}
	}

	// Rewind before emission; all input validation is complete.
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader = tar.NewReader(tmp)

	limited := &workspaceSnapshotLimitWriterV3{
		dst:       w,
		remaining: MaxCapsuleSizeV3,
	}
	writer := tar.NewWriter(limited)

	writeHeader := func(name string, size int64, mode int64) error {
		return writer.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     mode,
			Size:     size,
			Typeflag: tar.TypeReg,
		})
	}

	if err := writeHeader(
		"manifest.json", int64(len(manifestJSON)), 0600,
	); err != nil {
		return err
	}
	if _, err := writer.Write(manifestJSON); err != nil {
		return err
	}

	// Skip the original manifest in the intermediate archive.
	if _, err := reader.Next(); err != nil {
		return err
	}

	for _, metadata := range derived.Workspace.Files {
		sourceHeader, err := reader.Next()
		if err != nil {
			return err
		}

		name := "workspace/" + metadata.Path
		if sourceHeader.Name != name ||
			sourceHeader.Typeflag != tar.TypeReg ||
			sourceHeader.Size != metadata.Size {
			return fmt.Errorf(
				"capsule: intermediate v4 workspace mismatch: %s",
				name,
			)
		}

		if err := writeHeader(
			name, metadata.Size, sourceHeader.Mode,
		); err != nil {
			return err
		}
		if _, err := io.CopyN(
			writer, reader, metadata.Size,
		); err != nil {
			return err
		}
	}

	if _, err := reader.Next(); err != io.EOF {
		return fmt.Errorf(
			"capsule: unexpected intermediate workspace entry: %v",
			err,
		)
	}

	for _, mod := range derived.GoModules {
		for _, artifact := range mod.Artifacts {
			file, err := store.Open(artifact.Path)
			if err != nil {
				return fmt.Errorf(
					"capsule: reopen verified artifact %q: %w",
					artifact.Path, err,
				)
			}

			name := "dependencies/" + artifact.Path
			if err := writeHeader(name, artifact.Size, 0600); err != nil {
				_ = file.Close()
				return err
			}

			hash := sha256.New()
			written, copyErr := io.CopyN(
				io.MultiWriter(writer, hash),
				file,
				artifact.Size,
			)
			closeErr := file.Close()

			if copyErr != nil {
				return fmt.Errorf(
					"capsule: stream artifact %q: %w",
					artifact.Path, copyErr,
				)
			}
			if closeErr != nil {
				return closeErr
			}
			if written != artifact.Size {
				return fmt.Errorf(
					"capsule: short artifact write %q", artifact.Path,
				)
			}

			digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
			if !strings.EqualFold(digest, artifact.SHA256) {
				return fmt.Errorf(
					"capsule: changed artifact while writing %q",
					artifact.Path,
				)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("capsule: finalize mixed v4 TAR: %w", err)
	}
	return nil
}
