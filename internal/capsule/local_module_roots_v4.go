package capsule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveLocalModuleRootV4 validates the supported MVP layout:
// two sibling Go modules under the same physical parent directory.
//
// The returned path is not a pinned filesystem handle. Callers must
// not treat this check as protection against concurrent path swaps.
func ResolveLocalModuleRootV4(
	mainRoot string,
	replacePath string,
) (string, error) {
	if mainRoot == "" || !filepath.IsAbs(mainRoot) {
		return "", fmt.Errorf("capsule: main module root must be absolute")
	}

	if filepath.Clean(mainRoot) != mainRoot {
		return "", fmt.Errorf("capsule: noncanonical main module root")
	}

	if strings.ContainsRune(mainRoot, 0) {
		return "", fmt.Errorf("capsule: invalid main module root")
	}

	// Initial v4 MVP supports exactly one sibling-directory
	// replacement, such as ../lib.
	if !strings.HasPrefix(replacePath, "../") {
		return "", fmt.Errorf("capsule: replacement must be a sibling directory")
	}

	sibling := strings.TrimPrefix(replacePath, "../")

	if sibling == "" ||
		sibling == "." ||
		sibling == ".." ||
		strings.Contains(sibling, "/") ||
		strings.ContainsRune(sibling, '\\') ||
		strings.ContainsRune(sibling, 0) {
		return "", fmt.Errorf("capsule: invalid sibling module name")
	}

	parent := filepath.Dir(mainRoot)

	// Reject symlink roots before computing the sibling target.
	// This is a preflight check, not an atomic filesystem guarantee.
	for _, candidate := range []string{parent, mainRoot} {
		info, err := os.Lstat(candidate)
		if err != nil {
			return "", fmt.Errorf("capsule: inspect module directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("capsule: invalid module directory: %s", candidate)
		}
	}

	localRoot := filepath.Join(parent, sibling)

	if localRoot == mainRoot {
		return "", fmt.Errorf("capsule: replacement overlaps main module")
	}

	info, err := os.Lstat(localRoot)
	if err != nil {
		return "", fmt.Errorf("capsule: inspect local module root: %w", err)
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("capsule: local module root must be a real directory")
	}

	return localRoot, nil
}
