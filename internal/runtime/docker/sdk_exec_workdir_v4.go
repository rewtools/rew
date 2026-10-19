package docker

import "fmt"

const (
	snapshotExecWorkingDirV3 = "/workspace"
	snapshotExecWorkingDirV4 = "/workspace/app"
)

// validateSnapshotExecWorkingDirV4 restricts Docker Exec to the
// existing V3 workspace or the schema-V4 main module.
//
// The sibling module is accessible through relative Go module
// replacement but is not an allowed execution working directory.
func validateSnapshotExecWorkingDirV4(directory string) error {
	switch directory {
	case snapshotExecWorkingDirV3, snapshotExecWorkingDirV4:
		return nil
	default:
		return fmt.Errorf(
			"docker SDK: unsupported snapshot exec working directory %q",
			directory,
		)
	}
}
