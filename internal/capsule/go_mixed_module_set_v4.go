package capsule

import (
	"fmt"

	"golang.org/x/mod/module"
)

// ValidateMixedGoModuleSetV4 validates the identity and completeness
// of the initial mixed-module MVP:
//
//   - exactly one effective local directory replacement;
//   - exactly one external selected module;
//   - exactly one corresponding external artifact descriptor.
//
// It does not verify artifact files, ZIP contents, go.sum hashes,
// or the completeness of the Go toolchain's selected module graph.
//
// Callers must perform those checks independently before enabling
// mixed-dependency capture, capsule verification, or Docker replay.
func ValidateMixedGoModuleSetV4(
	selected []GoModuleIdentityV4,
	full []GoModuleV3,
) error {
	if len(selected) != 2 {
		return fmt.Errorf(
			"capsule: v4 mixed MVP requires exactly two selected modules",
		)
	}

	if len(full) != 1 {
		return fmt.Errorf(
			"capsule: v4 mixed MVP requires exactly one external module",
		)
	}

	seen := make(map[string]bool, len(selected))

	localCount := 0
	externalCount := 0

	var external GoModuleIdentityV4
	var previousPath string

	for _, item := range selected {
		if err := module.Check(item.Path, item.Version); err != nil {
			return fmt.Errorf(
				"capsule: invalid v4 selected module: %w",
				err,
			)
		}

		if seen[item.Path] {
			return fmt.Errorf(
				"capsule: duplicate v4 selected module: %s",
				item.Path,
			)
		}

		if previousPath != "" && item.Path <= previousPath {
			return fmt.Errorf(
				"capsule: v4 selected modules must be sorted by path",
			)
		}

		seen[item.Path] = true
		previousPath = item.Path

		if item.Replace != nil {
			localCount++

			if item.Replace.Path != "../lib" ||
				item.Replace.SnapshotPath != "lib" {
				return fmt.Errorf(
					"capsule: unsupported v4 local replacement",
				)
			}

			continue
		}

		externalCount++
		external = item
	}

	if localCount != 1 || externalCount != 1 {
		return fmt.Errorf(
			"capsule: v4 mixed MVP requires one local and one external module",
		)
	}

	artifact := full[0]

	if err := module.Check(artifact.Path, artifact.Version); err != nil {
		return fmt.Errorf(
			"capsule: invalid external artifact identity: %w",
			err,
		)
	}

	if artifact.Path != external.Path ||
		artifact.Version != external.Version {
		return fmt.Errorf(
			"capsule: external artifact does not match selected module",
		)
	}

	return nil
}
