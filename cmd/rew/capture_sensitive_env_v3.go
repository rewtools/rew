package main

import (
	"fmt"
	"strings"
)

var sensitiveEnvironmentNamesV3 = map[string]struct{}{
	"GITHUB_TOKEN":          {},
	"GH_TOKEN":              {},
	"GITLAB_TOKEN":          {},
	"AWS_SECRET_ACCESS_KEY": {},
	"AWS_SESSION_TOKEN":     {},
}

// checkSensitiveEnvironmentV3 rejects explicitly recognized,
// non-empty credential variables before schema-v3 capture.
//
// This is a conservative preflight check, not comprehensive
// secret detection. Values must never appear in returned errors.
func checkSensitiveEnvironmentV3(env []string) error {
	for _, entry := range env {
		name, value, valid := strings.Cut(entry, "=")
		if !valid || value == "" {
			continue
		}

		normalizedName := strings.ToUpper(name)

		if _, sensitive := sensitiveEnvironmentNamesV3[normalizedName]; sensitive {
			return fmt.Errorf(
				"sensitive environment variable %s is set; run capture without this credential",
				normalizedName,
			)
		}
	}

	return nil
}
