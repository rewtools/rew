package capsule

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// openPinnedRegularFileV4 opens a regular file by traversing one
// directory component at a time through pinned os.Root handles.
//
// It rejects symlinks in the inspected path components and verifies
// that every opened object matches the inspected directory entry.
//
// This does not make concurrent filesystem modifications atomic.
func openPinnedRegularFileV4(
	root *os.Root,
	name string,
) (result *os.File, err error) {
	if root == nil {
		return nil, fmt.Errorf("capsule: nil pinned file root")
	}
	if err := ValidateWorkspacePath(name, 0600); err != nil {
		return nil, fmt.Errorf("capsule: invalid file path: %w", err)
	}

	parts := strings.Split(name, "/")
	if len(parts) == 0 {
		return nil, fmt.Errorf("capsule: empty file path")
	}

	current := root
	var openedRoots []*os.Root

	defer func() {
		var closeErr error
		for i := len(openedRoots) - 1; i >= 0; i-- {
			closeErr = errors.Join(closeErr, openedRoots[i].Close())
		}
		if closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("capsule: close pinned directories: %w", closeErr),
			)
		}
		if err != nil && result != nil {
			err = errors.Join(err, result.Close())
			result = nil
		}
	}()

	for _, component := range parts[:len(parts)-1] {
		before, statErr := current.Lstat(component)
		if statErr != nil {
			return nil, fmt.Errorf(
				"capsule: inspect directory %s: %w",
				component, statErr,
			)
		}
		if !before.IsDir() ||
			before.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"capsule: invalid directory component: %s",
				component,
			)
		}

		child, openErr := current.OpenRoot(component)
		if openErr != nil {
			return nil, fmt.Errorf(
				"capsule: open directory %s: %w",
				component, openErr,
			)
		}
		openedRoots = append(openedRoots, child)

		opened, statErr := child.Stat(".")
		if statErr != nil || !os.SameFile(before, opened) {
			return nil, fmt.Errorf(
				"capsule: directory component changed: %s",
				component,
			)
		}

		after, statErr := current.Lstat(component)
		if statErr != nil || !after.IsDir() ||
			after.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(after, opened) {
			return nil, fmt.Errorf(
				"capsule: directory component replaced: %s",
				component,
			)
		}

		current = child
	}

	finalName := parts[len(parts)-1]

	before, err := current.Lstat(finalName)
	if err != nil {
		return nil, fmt.Errorf(
			"capsule: inspect file %s: %w", name, err,
		)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf(
			"capsule: nonregular file: %s", name,
		)
	}

	file, err := current.OpenFile(finalName, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf(
			"capsule: open file %s: %w", name, err,
		)
	}

	opened, statErr := file.Stat()
	if statErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(before, opened) {
		_ = file.Close()
		return nil, fmt.Errorf(
			"capsule: file changed during opening: %s", name,
		)
	}

	after, statErr := current.Lstat(finalName)
	if statErr != nil || !after.Mode().IsRegular() ||
		!os.SameFile(after, opened) {
		_ = file.Close()
		return nil, fmt.Errorf(
			"capsule: file replaced after opening: %s", name,
		)
	}

	return file, nil
}
