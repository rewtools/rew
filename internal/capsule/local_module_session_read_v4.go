package capsule

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
)

func errTooManyLocalModuleFilesV4() error {
	return fmt.Errorf("capsule: too many v4 module files")
}

// Read reads both modules through the handles owned by this session.
// No module directory is reopened by its physical pathname.
func (s *PinnedLocalModuleSessionV4) Read(
	mainPaths []string,
	localPaths []string,
) ([]WorkspaceFile, error) {
	if s == nil {
		return nil, fmt.Errorf("capsule: nil pinned module session")
	}
	if err := s.VerifyRootIdentity(); err != nil {
		return nil, err
	}

	if len(mainPaths) > MaxWorkspaceV3Files ||
		len(localPaths) > MaxWorkspaceV3Files-len(mainPaths) {
		return nil, errTooManyLocalModuleFilesV4()
	}

	var combinedSize int64
	mainFiles, err := readPinnedModuleFilesV4(
		s.main, mainPaths, &combinedSize,
	)
	if err != nil {
		return nil, fmt.Errorf("capsule: read main module: %w", err)
	}

	localFiles, err := readPinnedModuleFilesV4(
		s.local, localPaths, &combinedSize,
	)
	if err != nil {
		return nil, fmt.Errorf("capsule: read local module: %w", err)
	}

	if err := s.VerifyRootIdentity(); err != nil {
		return nil, err
	}

	return MapLocalWorkspaceV4(
		"app", mainFiles,
		"lib", localFiles,
	)
}

func readPinnedModuleFilesV4(
	root *os.Root,
	paths []string,
	total *int64,
) ([]WorkspaceFile, error) {
	if root == nil || total == nil {
		return nil, fmt.Errorf("capsule: invalid pinned module reader")
	}

	files := make([]WorkspaceFile, 0, len(paths))

	for _, name := range paths {
		if err := ValidateWorkspacePath(name, 0600); err != nil {
			return nil, fmt.Errorf(
				"capsule: invalid module file path: %w", err,
			)
		}

		// Traverse each directory through a pinned root.
		// The opener verifies that the final object is a regular file
		// and matches its inspected directory entry.
		file, err := openPinnedRegularFileV4(root, name)
		if err != nil {
			return nil, fmt.Errorf(
				"capsule: open pinned module file %s: %w", name, err,
			)
		}

		opened, statErr := file.Stat()
		if statErr != nil ||
			!opened.Mode().IsRegular() ||
			*total < 0 ||
			*total > MaxWorkspaceV2TotalSize ||
			opened.Size() < 0 ||
			opened.Size() > MaxWorkspaceFileSize ||
			opened.Size() > MaxWorkspaceV2TotalSize-*total {
			_ = file.Close()
			return nil, fmt.Errorf(
				"capsule: module file changed or exceeds budget: %s", name,
			)
		}

		data, readErr := io.ReadAll(
			io.LimitReader(file, MaxWorkspaceFileSize+1),
		)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return nil, fmt.Errorf(
				"capsule: read module file %s: read=%v close=%v",
				name, readErr, closeErr,
			)
		}

		if int64(len(data)) > MaxWorkspaceFileSize ||
			int64(len(data)) > MaxWorkspaceV2TotalSize-*total {
			return nil, fmt.Errorf("capsule: module file budget exceeded")
		}

		*total += int64(len(data))
		files = append(files, WorkspaceFile{
			Path: name,
			Data: data,
			Mode: 0600,
		})
	}

	return files, nil
}

// Verify compares a fresh read against the expected snapshot while
// retaining the same pinned physical module handles.
//
// It detects changes at verification time, not every possible
// intermediate mutation of a concurrently writable filesystem.
func (s *PinnedLocalModuleSessionV4) Verify(
	mainPaths []string,
	localPaths []string,
	expected []WorkspaceFile,
) error {
	if s == nil {
		return fmt.Errorf("capsule: nil pinned module session")
	}
	if err := s.VerifyRootIdentity(); err != nil {
		return err
	}

	if len(mainPaths) > MaxWorkspaceV3Files ||
		len(localPaths) > MaxWorkspaceV3Files-len(mainPaths) ||
		len(expected) == 0 ||
		len(expected) != len(mainPaths)+len(localPaths) {
		return fmt.Errorf("capsule: invalid expected v4 workspace file count")
	}

	seen := make(map[string]bool, len(expected))
	for i, file := range expected {
		var wanted string
		if i < len(mainPaths) {
			wanted = path.Join("app", mainPaths[i])
		} else {
			wanted = path.Join("lib", localPaths[i-len(mainPaths)])
		}

		if file.Path != wanted || seen[file.Path] {
			return fmt.Errorf(
				"capsule: invalid expected v4 workspace path: %s",
				file.Path,
			)
		}
		seen[file.Path] = true

		if err := ValidateWorkspacePath(file.Path, file.Mode); err != nil {
			return fmt.Errorf(
				"capsule: invalid expected v4 file metadata: %w", err,
			)
		}
	}

	current, err := s.Read(mainPaths, localPaths)
	if err != nil {
		return fmt.Errorf("capsule: reread pinned v4 modules: %w", err)
	}
	if len(current) != len(expected) {
		return fmt.Errorf("capsule: v4 workspace file count changed")
	}

	for i := range expected {
		a, b := expected[i], current[i]
		if a.Path != b.Path || a.Mode != b.Mode ||
			!bytes.Equal(a.Data, b.Data) {
			return fmt.Errorf(
				"capsule: v4 workspace file changed: %s", a.Path,
			)
		}
	}

	return s.VerifyRootIdentity()
}
