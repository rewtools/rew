package replay

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rewtools/rew/internal/capsule"
)

// prepareSnapshotFilesFromStoreV4 constructs both TARs before
// returning any readers. It owns the temporary files until
// the caller closes the returned snapshotFilesV3.
//
// A failed preparation closes all open files and removes the
// complete temporary directory, including partial TARs.
func prepareSnapshotFilesFromStoreV4(
	verified capsule.VerifiedStoredCapsuleV4,
) (*snapshotFilesV3, error) {
	return prepareSnapshotFilesFromStoreV4WithWriters(
		verified,
		func(w io.Writer) error {
			return capsule.WriteWorkspaceSnapshotFromStoreV4(
				w, verified,
			)
		},
		func(w io.Writer) error {
			return capsule.WriteGoModuleSnapshotFromStoreV4(
				w, verified,
			)
		},
	)
}

// prepareSnapshotFilesFromStoreV4WithWriters keeps the same
// resource ownership and failure cleanup for both TAR writers.
// Writer callbacks are used for deterministic failure testing.
func prepareSnapshotFilesFromStoreV4WithWriters(
	verified capsule.VerifiedStoredCapsuleV4,
	workspaceWriter func(io.Writer) error,
	moduleWriter func(io.Writer) error,
) (_ *snapshotFilesV3, resultErr error) {
	directory, err := os.MkdirTemp("", "rew-replay-v4-*")
	if err != nil {
		return nil, fmt.Errorf(
			"replay: create v4 snapshot directory: %w", err,
		)
	}

	result := &snapshotFilesV3{
		directory: directory,
	}

	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, result.Close())
		}
	}()

	writeArchive := func(
		name string,
		build func(io.Writer) error,
	) (*os.File, error) {
		filename := filepath.Join(directory, name)

		file, err := os.OpenFile(
			filename,
			os.O_RDWR|os.O_CREATE|os.O_EXCL,
			0600,
		)
		if err != nil {
			return nil, err
		}

		if err := build(file); err != nil {
			return nil, errors.Join(err, file.Close())
		}

		info, err := file.Stat()
		if err != nil {
			return nil, errors.Join(err, file.Close())
		}

		if !info.Mode().IsRegular() ||
			info.Size() < 1024 ||
			info.Size() > capsule.MaxCapsuleSizeV3 {
			return nil, errors.Join(
				fmt.Errorf(
					"replay: invalid v4 snapshot TAR size: %s",
					name,
				),
				file.Close(),
			)
		}

		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, errors.Join(err, file.Close())
		}

		return file, nil
	}

	result.workspace, err = writeArchive(
		"workspace.tar", workspaceWriter,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"replay: prepare mixed v4 workspace TAR: %w",
			err,
		)
	}

	result.modules, err = writeArchive(
		"modules.tar", moduleWriter,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"replay: prepare mixed v4 module TAR: %w",
			err,
		)
	}

	return result, nil
}
