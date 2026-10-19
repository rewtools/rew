package main

import (
	"fmt"

	"github.com/rewtools/rew/internal/capsule"
	"golang.org/x/mod/module"
)

// classifyCaptureModuleGraphV4 identifies the single local module
// and optional external module without depending on slice order.
//
// externalIndex is -1 for the legacy local-only profile.
// This function does not verify go.mod, go.sum or artifact bytes.
func classifyCaptureModuleGraphV4(
	selected []capsule.GoModuleIdentityV4,
	replacePath string,
) (localIndex, externalIndex int, err error) {
	localIndex = -1
	externalIndex = -1

	if replacePath != "../lib" {
		return -1, -1, fmt.Errorf(
			"capture v4: unsupported local replacement path %q",
			replacePath,
		)
	}

	if len(selected) < 1 || len(selected) > 2 {
		return -1, -1, fmt.Errorf(
			"capture v4: requires one local module and at most one external module",
		)
	}

	seen := make(map[string]bool, len(selected))

	for i, mod := range selected {
		if mod.Path == "" || seen[mod.Path] {
			return -1, -1, fmt.Errorf(
				"capture v4: missing or duplicate module identity",
			)
		}
		seen[mod.Path] = true

		if mod.Replace != nil {
			if localIndex != -1 {
				return -1, -1, fmt.Errorf(
					"capture v4: multiple local replacements",
				)
			}

			if mod.Version != "v0.0.0" ||
				mod.Replace.Path != replacePath ||
				(mod.Replace.SnapshotPath != "" &&
					mod.Replace.SnapshotPath != "lib") {
				return -1, -1, fmt.Errorf(
					"capture v4: unsupported local module replacement",
				)
			}

			if err := module.CheckPath(mod.Path); err != nil {
				return -1, -1, fmt.Errorf(
					"capture v4: invalid local module path: %w", err,
				)
			}

			localIndex = i
			continue
		}

		if externalIndex != -1 {
			return -1, -1, fmt.Errorf(
				"capture v4: multiple external modules",
			)
		}

		if err := module.Check(mod.Path, mod.Version); err != nil {
			return -1, -1, fmt.Errorf(
				"capture v4: invalid external module identity: %w", err,
			)
		}

		externalIndex = i
	}

	if localIndex == -1 {
		return -1, -1, fmt.Errorf(
			"capture v4: missing local replacement",
		)
	}

	return localIndex, externalIndex, nil
}
