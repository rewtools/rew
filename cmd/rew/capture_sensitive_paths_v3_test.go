package main

import (
	"strings"
	"testing"
)

func TestCheckSensitiveWorkspacePathsV3(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		blocked bool
	}{
		{"dot env", ".env", true},
		{"production env", ".env.production", true},
		{"local env nested", "config/.env.local", true},
		{"npm credentials", ".npmrc", true},
		{"nested npm credentials", "frontend/.npmrc", true},
		{"credentials json", "config/credentials.json", true},
		{"private key", "server.key", true},
		{"nested private key", "certs/server.key", true},
		{"ssh private key", "certs/id_rsa", true},
		{"aws credentials", "nested/.aws/credentials", true},
		{"case insensitive", "CONFIG/.ENV.PRODUCTION", true},

		{"env example", ".env.example", false},
		{"env sample", ".env.sample", false},
		{"nested env example", "config/.env.example", false},
		{"go source", "src/main.go", false},
		{"go module", "go.mod", false},
		{"go checksum", "go.sum", false},
		{"public key", "public.key", false},
		{"readme", "README.md", false},
		{"credentials example", "config/credentials.example.json", false},
		{"ordinary config", "config/settings.json", false},
		{"word secret in filename", "src/secret_handler.go", false},
		{"env backup", ".env.backup", true},
		{"env uppercase", ".ENV", true},
		{"env nested uppercase", "Config/.ENV.LOCAL", true},
		{"nested ssh key", "deploy/keys/id_ed25519", true},
		{"aws credentials uppercase", "config/.AWS/CREDENTIALS", true},
		{"nested private key uppercase", "certs/SERVER.KEY", true},
		{"nested public key", "certs/public.key", false},
		{"env example uppercase", ".ENV.EXAMPLE", false},
		{"env sample uppercase", ".ENV.SAMPLE", false},
		{"env template", ".env.template", true},
		{"credentials yaml", "config/credentials.yaml", false},
		{"ordinary key material", "src/keyboard.go", false},
		{"ordinary config file", "config/app.yaml", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSensitiveWorkspacePathsV3([]string{tc.path})

			if tc.blocked && err == nil {
				t.Fatalf("expected rejection for %q", tc.path)
			}

			if !tc.blocked && err != nil {
				t.Fatalf("unexpected rejection for %q: %v", tc.path, err)
			}

			if err != nil && !strings.Contains(err.Error(), "sensitive workspace path") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestCheckSensitiveWorkspacePathsV3MultipleFiles(t *testing.T) {
	if err := checkSensitiveWorkspacePathsV3(
		[]string{"go.mod", "src/main.go", "go.sum"},
	); err != nil {
		t.Fatalf("ordinary Go workspace rejected: %v", err)
	}

	if err := checkSensitiveWorkspacePathsV3(
		[]string{"go.mod", "config/.env.production", "src/main.go"},
	); err == nil {
		t.Fatal("expected rejection of sensitive file in mixed workspace")
	}
}

func TestCheckSensitiveWorkspacePathsV3EmptyList(t *testing.T) {
	if err := checkSensitiveWorkspacePathsV3(nil); err != nil {
		t.Fatalf("empty path list rejected: %v", err)
	}
}
