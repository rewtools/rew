package capsule

import (
	"fmt"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// ValidateGoModuleConsistencyV4 validates the supported MVP
// relationship between go.mod and the selected Go module graph.
//
// This checks metadata consistency only. It does not validate
// actual workspace file bytes, source-tree completeness, or
// external module cache artifacts.
func ValidateGoModuleConsistencyV4(
	goMod []byte,
	selected []GoModuleIdentityV4,
) error {
	if len(goMod) == 0 || int64(len(goMod)) > MaxWorkspaceFileSize {
		return fmt.Errorf("capsule: invalid v4 go.mod size")
	}

	parsed, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil {
		return fmt.Errorf("capsule: parse v4 go.mod: %w", err)
	}

	if parsed.Module == nil {
		return fmt.Errorf("capsule: missing v4 main module declaration")
	}

	if err := module.CheckPath(parsed.Module.Mod.Path); err != nil {
		return fmt.Errorf("capsule: invalid v4 main module: %w", err)
	}

	if len(parsed.Exclude) != 0 {
		return fmt.Errorf("capsule: v4 MVP does not support exclude")
	}

	if len(parsed.Replace) != 1 {
		return fmt.Errorf("capsule: v4 MVP requires one replace directive")
	}

	replacement := parsed.Replace[0]

	if replacement.Old.Version != "" ||
		replacement.New.Version != "" {
		return fmt.Errorf("capsule: v4 MVP does not support versioned replace")
	}

	if err := module.CheckPath(replacement.Old.Path); err != nil {
		return fmt.Errorf("capsule: invalid replacement module path: %w", err)
	}

	required := make(map[string]string, len(parsed.Require))

	for _, entry := range parsed.Require {
		if err := module.Check(entry.Mod.Path, entry.Mod.Version); err != nil {
			return fmt.Errorf("capsule: invalid required module: %w", err)
		}

		if _, exists := required[entry.Mod.Path]; exists {
			return fmt.Errorf("capsule: duplicate required module")
		}

		required[entry.Mod.Path] = entry.Mod.Version
	}

	requiredVersion, exists := required[replacement.Old.Path]
	if !exists {
		return fmt.Errorf("capsule: replaced module missing from require")
	}

	if len(selected) == 0 || len(selected) > MaxGoSelectedModulesV3 {
		return fmt.Errorf("capsule: invalid v4 selected graph")
	}

	seen := make(map[string]bool, len(selected))
	localMatches := 0

	for _, item := range selected {
		if err := module.Check(item.Path, item.Version); err != nil {
			return fmt.Errorf("capsule: invalid selected module: %w", err)
		}

		if seen[item.Path] {
			return fmt.Errorf("capsule: duplicate selected module")
		}
		seen[item.Path] = true

		if item.Replace == nil {
			continue
		}

		localMatches++

		if item.Path != replacement.Old.Path ||
			semver.Compare(item.Version, requiredVersion) < 0 ||
			item.Replace.Path != replacement.New.Path {
			return fmt.Errorf(
				"capsule: go.mod and effective replacement graph mismatch",
			)
		}
	}

	if localMatches != 1 {
		return fmt.Errorf("capsule: expected one effective local replacement")
	}

	// Every required module must appear in the selected build list.
	// MVS may raise external module versions, but cannot select a
	// version lower than the corresponding go.mod requirement.
	selectedVersions := make(map[string]string, len(selected))
	for _, item := range selected {
		selectedVersions[item.Path] = item.Version
	}

	for requiredPath, requiredVersion := range required {
		selectedVersion, ok := selectedVersions[requiredPath]
		if !ok {
			return fmt.Errorf(
				"capsule: required module missing from selected graph: %s",
				requiredPath,
			)
		}

		if semver.Compare(selectedVersion, requiredVersion) < 0 {
			return fmt.Errorf(
				"capsule: selected module version below requirement: %s",
				requiredPath,
			)
		}
	}

	return nil
}
