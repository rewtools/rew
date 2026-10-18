package main

import (
	"fmt"
	"path"
	"strings"
)

// checkSensitiveWorkspacePathsV3 rejects a narrow set of
// high-confidence sensitive workspace filenames.
//
// It examines names only, never reads file contents.
// It is not comprehensive secret detection or redaction.
func checkSensitiveWorkspacePathsV3(paths []string) error {
	for _, name := range paths {
		if isSensitiveWorkspacePathV3(name) {
			return fmt.Errorf(
				"sensitive workspace path %q is selected; remove it from capture inputs",
				name,
			)
		}
	}
	return nil
}

func isSensitiveWorkspacePathV3(name string) bool {
	normalized := strings.ToLower(name)
	base := path.Base(normalized)

	switch base {
	case ".npmrc",
		"credentials.json",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa":
		return true
	case ".env.example", ".env.sample", "public.key":
		return false
	}

	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}

	if strings.HasSuffix(base, ".key") {
		return true
	}

	parts := strings.Split(normalized, "/")
	for i := 1; i < len(parts); i++ {
		if parts[i-1] == ".aws" && parts[i] == "credentials" {
			return true
		}
	}

	return false
}
