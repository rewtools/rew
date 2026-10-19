package capsule

import (
	"fmt"
	"sort"
)

// MergeLocalGoSourcesV4 combines already discovered module-relative
// source and embed paths into two canonical, sorted file lists.
//
// Each module requires its own go.mod. Files are deduplicated within
// their module, but the combined workspace file limit is shared.
func MergeLocalGoSourcesV4(
	mainSources []string,
	localSources []string,
) ([]string, []string, error) {
	mainFiles := make(map[string]struct{})
	localFiles := make(map[string]struct{})

	total := 0

	add := func(
		files map[string]struct{},
		name string,
	) error {
		// Validate before deduplication or path normalization.
		if err := ValidateWorkspacePath(name, 0600); err != nil {
			return fmt.Errorf(
				"capsule: invalid discovered Go file %q: %w",
				name, err,
			)
		}

		if _, exists := files[name]; exists {
			return nil
		}

		if total >= MaxWorkspaceV3Files {
			return fmt.Errorf(
				"capsule: too many discovered v4 workspace files",
			)
		}

		files[name] = struct{}{}
		total++
		return nil
	}

	// Reserve mandatory module manifests in the shared budget.
	if err := add(mainFiles, "go.mod"); err != nil {
		return nil, nil, err
	}
	if err := add(localFiles, "go.mod"); err != nil {
		return nil, nil, err
	}

	for _, name := range mainSources {
		if err := add(mainFiles, name); err != nil {
			return nil, nil, err
		}
	}

	for _, name := range localSources {
		if err := add(localFiles, name); err != nil {
			return nil, nil, err
		}
	}

	sorted := func(files map[string]struct{}) []string {
		result := make([]string, 0, len(files))

		for name := range files {
			result = append(result, name)
		}

		sort.Strings(result)
		return result
	}

	return sorted(mainFiles), sorted(localFiles), nil
}
