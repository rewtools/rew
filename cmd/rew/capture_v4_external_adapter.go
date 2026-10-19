package main

import (
	"github.com/rewtools/rew/internal/capsule"
)

// captureExternalModuleIdentitiesV4 selects the external module
// identities needed by the existing V3 artifact capture reader.
//
// A local replacement is never forwarded to the V3 reader.
// The empty result preserves the local-only V4 profile.
//
// This is identity conversion, not artifact verification.
func captureExternalModuleIdentitiesV4(
	selected []capsule.GoModuleIdentityV4,
	replacePath string,
) ([]capsule.GoModuleIdentityV3, error) {
	_, externalIndex, err := classifyCaptureModuleGraphV4(
		selected, replacePath,
	)
	if err != nil {
		return nil, err
	}

	if externalIndex == -1 {
		return nil, nil
	}

	external := selected[externalIndex]

	return []capsule.GoModuleIdentityV3{
		{
			Path:    external.Path,
			Version: external.Version,
		},
	}, nil
}
