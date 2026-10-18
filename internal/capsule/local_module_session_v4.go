package capsule

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PinnedLocalModuleSessionV4 holds directory handles for a pair of
// sibling Go modules. The handles remain open until Close.
//
// Root identity checks detect directory-entry replacement at the
// moments of verification. They do not provide an atomic snapshot.
type PinnedLocalModuleSessionV4 struct {
	parentPath string
	mainName   string
	localName  string

	parent *os.Root
	main   *os.Root
	local  *os.Root
}

// OpenPinnedLocalModuleSessionV4 validates and pins both module
// directories relative to a pinned common parent directory.
func OpenPinnedLocalModuleSessionV4(
	mainRoot string,
	replacePath string,
) (_ *PinnedLocalModuleSessionV4, err error) {
	localRoot, err := ResolveLocalModuleRootV4(
		mainRoot, replacePath,
	)
	if err != nil {
		return nil, err
	}

	parentPath := filepath.Dir(mainRoot)
	mainName := filepath.Base(mainRoot)
	localName := filepath.Base(localRoot)

	parentBefore, err := os.Lstat(parentPath)
	if err != nil {
		return nil, fmt.Errorf("capsule: stat module parent: %w", err)
	}
	if !parentBefore.IsDir() ||
		parentBefore.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("capsule: invalid module parent")
	}

	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return nil, fmt.Errorf("capsule: open module parent: %w", err)
	}

	session := &PinnedLocalModuleSessionV4{
		parentPath: parentPath,
		mainName:   mainName,
		localName:  localName,
		parent:     parent,
	}

	defer func() {
		if err != nil {
			_ = session.Close()
		}
	}()

	openedParent, err := parent.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("capsule: inspect opened parent: %w", err)
	}
	if !os.SameFile(parentBefore, openedParent) {
		return nil, fmt.Errorf("capsule: module parent changed while opening")
	}

	openChild := func(name string) (*os.Root, error) {
		before, err := parent.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !before.IsDir() ||
			before.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("capsule: invalid module directory: %s", name)
		}

		root, err := parent.OpenRoot(name)
		if err != nil {
			return nil, err
		}

		opened, statErr := root.Stat(".")
		if statErr != nil {
			_ = root.Close()
			return nil, statErr
		}
		if !os.SameFile(before, opened) {
			_ = root.Close()
			return nil, fmt.Errorf("capsule: module directory changed: %s", name)
		}

		current, statErr := parent.Lstat(name)
		if statErr != nil {
			_ = root.Close()
			return nil, statErr
		}
		if !current.IsDir() ||
			current.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(current, opened) {
			_ = root.Close()
			return nil, fmt.Errorf("capsule: module directory replaced: %s", name)
		}

		return root, nil
	}

	session.main, err = openChild(mainName)
	if err != nil {
		return nil, fmt.Errorf("capsule: pin main module: %w", err)
	}

	session.local, err = openChild(localName)
	if err != nil {
		return nil, fmt.Errorf("capsule: pin local module: %w", err)
	}

	if err = session.VerifyRootIdentity(); err != nil {
		return nil, err
	}

	return session, nil
}

// VerifyRootIdentity checks whether the original module directory
// entries still refer to the directories pinned by this session.
func (s *PinnedLocalModuleSessionV4) VerifyRootIdentity() error {
	if s == nil || s.parent == nil ||
		s.main == nil || s.local == nil {
		return fmt.Errorf("capsule: pinned module session is closed")
	}

	parentOpened, err := s.parent.Stat(".")
	if err != nil {
		return fmt.Errorf("capsule: inspect pinned parent: %w", err)
	}

	parentCurrent, err := os.Lstat(s.parentPath)
	if err != nil {
		return fmt.Errorf("capsule: stat current parent: %w", err)
	}
	if !parentCurrent.IsDir() ||
		parentCurrent.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(parentCurrent, parentOpened) {
		return fmt.Errorf("capsule: pinned parent identity changed")
	}

	for _, item := range []struct {
		name string
		root *os.Root
	}{
		{name: s.mainName, root: s.main},
		{name: s.localName, root: s.local},
	} {
		opened, err := item.root.Stat(".")
		if err != nil {
			return fmt.Errorf("capsule: inspect pinned module: %w", err)
		}

		current, err := s.parent.Lstat(item.name)
		if err != nil {
			return fmt.Errorf(
				"capsule: stat current module %s: %w",
				item.name, err,
			)
		}

		if !current.IsDir() ||
			current.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(current, opened) {
			return fmt.Errorf(
				"capsule: pinned module identity changed: %s",
				item.name,
			)
		}
	}

	return nil
}

// Close releases all pinned directory handles.
func (s *PinnedLocalModuleSessionV4) Close() error {
	if s == nil {
		return nil
	}

	var errs []error

	for _, root := range []*os.Root{s.local, s.main, s.parent} {
		if root != nil {
			if err := root.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	s.local = nil
	s.main = nil
	s.parent = nil

	return errors.Join(errs...)
}
