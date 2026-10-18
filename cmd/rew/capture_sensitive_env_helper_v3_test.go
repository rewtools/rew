package main

import (
	"os"
	"testing"
)

// setupCaptureV3WithoutCredentials isolates capture tests from
// credentials inherited from developer and CI environments.
//
// This helper is test-only. Production capture always performs
// the sensitive environment preflight.
func setupCaptureV3WithoutCredentials(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"GITLAB_TOKEN",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
	} {
		t.Setenv(name, "")
	}
}

func TestSetupCaptureV3WithoutCredentials(t *testing.T) {
	names := []string{
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"GITLAB_TOKEN",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
	}

	t.Setenv("REW_TEST_ORDINARY_VARIABLE", "keep-this-value")

	for _, name := range names {
		t.Setenv(name, "synthetic-test-value")
	}

	if err := checkSensitiveEnvironmentV3(os.Environ()); err == nil {
		t.Fatal("expected credentials to be detected before setup")
	}

	setupCaptureV3WithoutCredentials(t)

	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			t.Fatalf("%s was not cleared", name)
		}
	}

	if err := checkSensitiveEnvironmentV3(os.Environ()); err != nil {
		t.Fatalf("credentials remain after setup: %v", err)
	}

	if got := os.Getenv("REW_TEST_ORDINARY_VARIABLE"); got != "keep-this-value" {
		t.Fatalf("ordinary variable changed: %q", got)
	}
}
