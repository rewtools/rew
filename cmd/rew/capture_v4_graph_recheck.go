package main

import "github.com/rewtools/rew/internal/capsule"

// captureV4GraphUnchanged compares the complete selected Go module
// graph before and after execution.
//
// The captured graph contains REW's own SnapshotPath annotation,
// while go list returns only the original local replacement path.
//
// Only that annotation difference is permitted.
func captureV4GraphUnchanged(
	before []capsule.GoModuleIdentityV4,
	after []capsule.GoModuleIdentityV4,
) bool {
	if len(before) == 0 || len(before) != len(after) {
		return false
	}

	beforeLocal, beforeExternal, err :=
		classifyCaptureModuleGraphV4(before, "../lib")
	if err != nil {
		return false
	}

	afterLocal, afterExternal, err :=
		classifyCaptureModuleGraphV4(after, "../lib")
	if err != nil {
		return false
	}

	if beforeLocal != afterLocal ||
		beforeExternal != afterExternal {
		return false
	}

	for i := range before {
		a := before[i]
		b := after[i]

		if a.Path != b.Path || a.Version != b.Version {
			return false
		}

		if (a.Replace == nil) != (b.Replace == nil) {
			return false
		}

		if a.Replace != nil {
			if a.Replace.Path != b.Replace.Path {
				return false
			}

			// Before capture may contain the packaged path,
			// but the actual Go graph must not.
			if a.Replace.SnapshotPath != "lib" ||
				b.Replace.SnapshotPath != "" {
				return false
			}
		}
	}

	return true
}
