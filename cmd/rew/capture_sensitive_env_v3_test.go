package main

import (
	"strings"
	"testing"
)

func TestCheckSensitiveEnvironmentV3(t *testing.T) {
	tests := []struct {
		name       string
		env        []string
		wantName   string
		wantReject bool
	}{
		{
			name: "ordinary environment",
			env: []string{
				"PATH=/usr/bin",
				"HOME=/tmp",
				"CI=true",
				"GOOS=linux",
			},
		},
		{
			name:       "github token",
			env:        []string{"GITHUB_TOKEN=secret-example-value"},
			wantName:   "GITHUB_TOKEN",
			wantReject: true,
		},
		{
			name:       "gh token",
			env:        []string{"GH_TOKEN=secret-example-value"},
			wantName:   "GH_TOKEN",
			wantReject: true,
		},
		{
			name:       "gitlab token",
			env:        []string{"GITLAB_TOKEN=secret-example-value"},
			wantName:   "GITLAB_TOKEN",
			wantReject: true,
		},
		{
			name:       "aws secret access key",
			env:        []string{"AWS_SECRET_ACCESS_KEY=secret-example-value"},
			wantName:   "AWS_SECRET_ACCESS_KEY",
			wantReject: true,
		},
		{
			name:       "aws session token",
			env:        []string{"AWS_SESSION_TOKEN=secret-example-value"},
			wantName:   "AWS_SESSION_TOKEN",
			wantReject: true,
		},
		{
			name: "case insensitive",
			env: []string{
				"github_token=secret-example-value",
			},
			wantName:   "GITHUB_TOKEN",
			wantReject: true,
		},
		{
			name: "empty sensitive value",
			env: []string{
				"GITHUB_TOKEN=",
				"AWS_SECRET_ACCESS_KEY=",
			},
		},
		{
			name: "unknown variable not blocked",
			env: []string{
				"CUSTOM_APP_SETTING=secret-example-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSensitiveEnvironmentV3(tt.env)

			if !tt.wantReject {
				if err != nil {
					t.Fatalf("unexpected rejection: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected sensitive environment rejection")
			}

			if !strings.Contains(err.Error(), tt.wantName) {
				t.Fatalf(
					"error does not identify variable: %q",
					err,
				)
			}

			if strings.Contains(err.Error(), "secret-example-value") {
				t.Fatal("error leaked sensitive variable value")
			}
		})
	}
}
