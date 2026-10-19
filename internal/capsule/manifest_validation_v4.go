package capsule

import (
	"fmt"

	"golang.org/x/mod/module"
)

// Validate checks the initial schema-v4 metadata contract.
//
// This is not capsule verification: it does not establish that
// files exist, their recorded digests match, or capture was
// complete and consistent.
func (m ManifestV4) Validate() error {
	if m.SchemaVersion != SchemaVersionV4 {
		return fmt.Errorf("capsule: unsupported schema version for v4 manifest")
	}

	if len(m.Execution.Argv) == 0 ||
		m.Execution.Argv[0] != "go" {
		return fmt.Errorf("capsule: invalid v4 execution command")
	}

	if m.Execution.ExitCode == 0 {
		return fmt.Errorf("capsule: v4 requires a failed execution")
	}

	if m.Platform.OS == "" || m.Platform.Architecture == "" {
		return fmt.Errorf("capsule: missing v4 platform")
	}

	if m.Runtime == nil ||
		m.Runtime.Kind != "go" ||
		!IsSupportedGoVersion(m.Runtime.Version) {
		return fmt.Errorf("capsule: unsupported v4 Go runtime")
	}

	if m.GoBuildEnv == nil ||
		m.GoBuildEnv.GOOS == "" ||
		m.GoBuildEnv.GOARCH == "" ||
		m.GoBuildEnv.CGOEnabled != "0" {
		return fmt.Errorf("capsule: invalid v4 Go build environment")
	}

	if err := validateV4SnapshotDirectory(m.Execution.WorkingDir); err != nil {
		return fmt.Errorf("capsule: invalid v4 working directory: %w", err)
	}

	// The first implementation slice supports exactly one
	// effective local replacement.
	if len(m.LocalModules) != 1 {
		return fmt.Errorf("capsule: v4 MVP requires exactly one local module")
	}

	if len(m.GoSelectedModules) == 0 ||
		len(m.GoSelectedModules) > MaxGoSelectedModulesV3 {
		return fmt.Errorf("capsule: invalid v4 selected module count")
	}

	local := m.LocalModules[0]

	if err := module.CheckPath(local.ModulePath); err != nil {
		return fmt.Errorf("capsule: invalid v4 local module path: %w", err)
	}

	if err := validateV4SnapshotDirectory(local.SnapshotPath); err != nil {
		return fmt.Errorf("capsule: invalid v4 local module snapshot path: %w", err)
	}

	seen := make(map[string]bool, len(m.GoSelectedModules))
	localMatches := 0

	for _, selected := range m.GoSelectedModules {
		if err := module.Check(selected.Path, selected.Version); err != nil {
			return fmt.Errorf("capsule: invalid v4 selected module: %w", err)
		}

		if seen[selected.Path] {
			return fmt.Errorf("capsule: duplicate v4 selected module: %s", selected.Path)
		}
		seen[selected.Path] = true

		if selected.Replace == nil {
			continue
		}

		localMatches++

		if selected.Path != local.ModulePath {
			return fmt.Errorf("capsule: replacement module identity mismatch")
		}

		if selected.Replace.SnapshotPath != local.SnapshotPath {
			return fmt.Errorf("capsule: replacement snapshot metadata mismatch")
		}

		if err := ValidateLocalReplacementPathV4(
			m.Execution.WorkingDir,
			selected.Replace.Path,
			selected.Replace.SnapshotPath,
		); err != nil {
			return err
		}
	}

	if localMatches != 1 {
		return fmt.Errorf("capsule: v4 MVP requires exactly one effective replacement")
	}

	if err := validateLocalModuleFilesV4(m); err != nil {
		return err
	}

	return nil
}
