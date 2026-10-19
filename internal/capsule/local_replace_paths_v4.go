package capsule

import (
	"fmt"
	"path"
	"strings"
)

// ValidateLocalReplacementPathV4 checks that a relative Go replacement
// resolves to the expected directory within a virtual snapshot root.
//
// This checks lexical paths only. Filesystem identity, symlinks, and
// actual source-tree containment must be verified separately.
func ValidateLocalReplacementPathV4(
	workingDir string,
	replacePath string,
	snapshotPath string,
) error {
	if err := validateV4SnapshotDirectory(workingDir); err != nil {
		return fmt.Errorf("capsule: invalid v4 working directory: %w", err)
	}

	if err := validateV4SnapshotDirectory(snapshotPath); err != nil {
		return fmt.Errorf("capsule: invalid v4 replacement snapshot path: %w", err)
	}

	if replacePath == "" ||
		strings.HasPrefix(replacePath, "/") ||
		strings.ContainsRune(replacePath, '\\') ||
		strings.ContainsRune(replacePath, 0) {
		return fmt.Errorf("capsule: invalid v4 relative replacement path")
	}

	// Parent components are permitted only as a leading prefix.
	// Reject redundant interior "." and ".." components before
	// path.Clean can silently normalize them.
	components := strings.Split(replacePath, "/")
	for index, component := range components {
		switch component {
		case "":
			return fmt.Errorf("capsule: empty v4 replacement path component")
		case ".":
			if index != 0 {
				return fmt.Errorf("capsule: noncanonical v4 replacement path")
			}
		case "..":
			if index > 0 && components[index-1] != ".." &&
				components[index-1] != "." {
				return fmt.Errorf("capsule: noncanonical v4 replacement path")
			}
		}
	}

	resolved := path.Clean(path.Join(workingDir, replacePath))

	if resolved == "." ||
		resolved == ".." ||
		strings.HasPrefix(resolved, "../") ||
		strings.HasPrefix(resolved, "/") {
		return fmt.Errorf("capsule: replacement escapes snapshot root")
	}

	if resolved != snapshotPath {
		return fmt.Errorf("capsule: replacement snapshot path mismatch")
	}

	if resolved == workingDir ||
		strings.HasPrefix(workingDir, resolved+"/") {
		return fmt.Errorf("capsule: replacement overlaps main module root")
	}

	return nil
}

func validateV4SnapshotDirectory(name string) error {
	if name == "" ||
		name == "." ||
		strings.HasPrefix(name, "/") ||
		strings.ContainsRune(name, '\\') ||
		strings.ContainsRune(name, 0) ||
		path.Clean(name) != name {
		return fmt.Errorf("invalid canonical relative directory")
	}

	for _, component := range strings.Split(name, "/") {
		if component == "" ||
			component == "." ||
			component == ".." {
			return fmt.Errorf("invalid snapshot directory component")
		}
	}

	return nil
}
