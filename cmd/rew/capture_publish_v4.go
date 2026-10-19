package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// publishCaptureV4 writes into a private temporary file in the
// destination directory, then publishes it without replacing an
// existing destination.
//
// The write callback must not retain the writer after returning.
func publishCaptureV4(
	output string,
	write func(io.Writer) error,
) error {
	if output == "" || write == nil {
		return fmt.Errorf("invalid capture publication input")
	}

	if err := checkCaptureOutputAvailableV3(output); err != nil {
		return err
	}

	dir := filepath.Dir(output)

	temp, err := os.CreateTemp(dir, ".rew-capture-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary capsule: %w", err)
	}

	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("secure temporary capsule: %w", err)
	}

	writeErr := write(temp)
	if writeErr != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary capsule: %w", writeErr)
	}

	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary capsule: %w", err)
	}

	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary capsule: %w", err)
	}

	// Atomic no-clobber publication. Unlike os.Rename, os.Link
	// fails if another process has already created output.
	if err := os.Link(tempPath, output); err != nil {
		return fmt.Errorf("publish capsule without overwrite: %w", err)
	}

	return nil
}
