package capsule

import "fmt"

// VerifyMixedGoModuleArtifactsV4 verifies the external Go module
// artifacts for the initial schema-v4 mixed-module MVP.
//
// The selected graph must contain exactly one local replacement
// and one external module. The external module must have the
// complete ZIP, mod, and info artifact set.
//
// This function reuses schema-v3 artifact verification, including
// ZIP integrity, module identity, go.sum checksums, and size limits.
//
// The caller must independently verify the provenance and
// integrity of goSum and the selected Go module graph.
//
// The artifact store must remain open and must not be modified
// concurrently during verification.
func VerifyMixedGoModuleArtifactsV4(
	selected []GoModuleIdentityV4,
	full []GoModuleV3,
	goSum []byte,
	store *ArtifactStoreV3,
) error {
	if err := ValidateMixedGoModuleSetV4(
		selected,
		full,
	); err != nil {
		return fmt.Errorf(
			"capsule: invalid v4 mixed module set: %w",
			err,
		)
	}

	if err := ValidateGoModulesV3(full); err != nil {
		return fmt.Errorf(
			"capsule: invalid v4 external module metadata: %w",
			err,
		)
	}

	if err := verifyGoModuleSetFromStoreV3(
		full,
		goSum,
		store,
	); err != nil {
		return fmt.Errorf(
			"capsule: invalid v4 external module artifacts: %w",
			err,
		)
	}

	return nil
}
