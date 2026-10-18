package main

import (
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
)

func TestAuditSensitiveDataV3(t *testing.T) {
	cases := []struct {
		name     string
		manifest capsule.Manifest
		want     string
	}{
		{
			name: "sensitive workspace filename",
			manifest: capsule.Manifest{
				Workspace: capsule.Workspace{
					Files: []capsule.FileMetadata{
						{Path: "go.mod"},
						{Path: "config/.env.production"},
					},
				},
			},
			want: "workspace path",
		},
		{
			name: "sensitive argv",
			manifest: capsule.Manifest{
				Execution: capsule.Execution{
					Argv: []string{
						"go",
						"build",
						"GITHUB_TOKEN=synthetic-test-secret",
					},
				},
			},
			want: "command argument",
		},
		{
			name: "sensitive stderr",
			manifest: capsule.Manifest{
				Execution: capsule.Execution{
					Stderr: "error: AWS_SECRET_ACCESS_KEY=synthetic-test-secret",
				},
			},
			want: "stderr",
		},
		{
			name: "sensitive stdout",
			manifest: capsule.Manifest{
				Execution: capsule.Execution{
					Stdout: "GH_TOKEN=synthetic-test-secret",
				},
			},
			want: "stdout",
		},
		{
			name: "ordinary Go metadata",
			manifest: capsule.Manifest{
				Workspace: capsule.Workspace{
					Files: []capsule.FileMetadata{
						{Path: "go.mod"},
						{Path: "main.go"},
					},
				},
				Execution: capsule.Execution{
					Argv:   []string{"go", "build", "./..."},
					Stderr: "undefined: missingSymbol",
				},
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := auditSensitiveDataV3(tc.manifest)

			if tc.want == "" {
				if len(findings) != 0 {
					t.Fatalf("unexpected findings: %v", findings)
				}
				return
			}

			if len(findings) == 0 {
				t.Fatal("expected sensitive-data finding")
			}

			found := false
			for _, finding := range findings {
				if strings.Contains(finding, tc.want) {
					found = true
				}
				if strings.Contains(finding, "synthetic-test-secret") {
					t.Fatal("finding leaked synthetic secret")
				}
			}

			if !found {
				t.Fatalf("expected finding category %q, got %v",
					tc.want, findings)
			}
		})
	}
}

func TestAuditSensitiveDataV3EdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		manifest capsule.Manifest
		wantHits int
	}{
		{
			name: "case insensitive assignment",
			manifest: capsule.Manifest{
				Execution: capsule.Execution{
					Stderr: "github_token=synthetic-test-secret",
				},
			},
			wantHits: 1,
		},
		{
			name: "empty credential value",
			manifest: capsule.Manifest{
				Execution: capsule.Execution{
					Stderr: "GITHUB_TOKEN=",
				},
			},
			wantHits: 0,
		},
		{
			name: "ordinary variable name",
			manifest: capsule.Manifest{
				Execution: capsule.Execution{
					Stderr: "MY_GITHUB_TOKEN=ordinary",
				},
			},
			wantHits: 0,
		},
		{
			name: "ordinary source filename",
			manifest: capsule.Manifest{
				Workspace: capsule.Workspace{
					Files: []capsule.FileMetadata{
						{Path: "config/.env.example"},
						{Path: "main.go"},
					},
				},
			},
			wantHits: 0,
		},
		{
			name: "multiple locations",
			manifest: capsule.Manifest{
				Workspace: capsule.Workspace{
					Files: []capsule.FileMetadata{
						{Path: "config/.env.production"},
					},
				},
				Execution: capsule.Execution{
					Argv:   []string{"GH_TOKEN=synthetic-test-secret"},
					Stdout: "GITHUB_TOKEN=synthetic-test-secret",
					Stderr: "AWS_SESSION_TOKEN=synthetic-test-secret",
				},
			},
			wantHits: 4,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := auditSensitiveDataV3(tc.manifest)

			if len(findings) != tc.wantHits {
				t.Fatalf("got %d findings, want %d: %v",
					len(findings), tc.wantHits, findings)
			}

			for _, finding := range findings {
				if strings.Contains(finding, "synthetic-test-secret") {
					t.Fatal("audit finding leaked a credential value")
				}
			}
		})
	}
}

func TestAuditWorkspaceContentV3(t *testing.T) {
	cases := []struct {
		name     string
		files    []capsule.WorkspaceFile
		wantHits int
	}{
		{
			name: "credential in ordinary source file",
			files: []capsule.WorkspaceFile{
				{
					Path: "main.go",
					Data: []byte(
						"const token = \"GITHUB_TOKEN=synthetic-test-secret\"\n",
					),
				},
			},
			wantHits: 1,
		},
		{
			name: "ordinary Go source",
			files: []capsule.WorkspaceFile{
				{
					Path: "main.go",
					Data: []byte("package main\nfunc main() {}\n"),
				},
			},
			wantHits: 0,
		},
		{
			name: "credential in two files",
			files: []capsule.WorkspaceFile{
				{
					Path: "config.txt",
					Data: []byte("GH_TOKEN=synthetic-test-secret\n"),
				},
				{
					Path: "settings.txt",
					Data: []byte("AWS_SESSION_TOKEN=synthetic-test-secret\n"),
				},
			},
			wantHits: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := auditWorkspaceContentV3(tc.files)

			if len(findings) != tc.wantHits {
				t.Fatalf("got %d findings, want %d: %v",
					len(findings), tc.wantHits, findings)
			}

			for _, finding := range findings {
				if !strings.Contains(
					finding,
					"workspace content: credential assignment",
				) {
					t.Fatalf("unexpected finding: %q", finding)
				}

				if strings.Contains(finding, "synthetic-test-secret") {
					t.Fatal("workspace finding leaked secret")
				}
			}
		})
	}
}

func TestAuditCredentialFormatsV3(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"plain assignment", "GITHUB_TOKEN=synthetic-secret", true},
		{"double quoted", `GITHUB_TOKEN="synthetic-secret"`, true},
		{"single quoted", "GITHUB_TOKEN='synthetic-secret'", true},
		{"YAML assignment", "GITHUB_TOKEN: synthetic-secret", true},
		{"JSON field", `"GITHUB_TOKEN": "synthetic-secret"`, true},
		{"export assignment", "export GITHUB_TOKEN=synthetic-secret", true},
		{"empty assignment", "GITHUB_TOKEN=", false},
		{"empty double quoted", `GITHUB_TOKEN=""`, false},
		{"empty single quoted", "GITHUB_TOKEN=''", false},
		{"empty YAML value", "GITHUB_TOKEN: ", false},
		{"empty JSON value", `"GITHUB_TOKEN": ""`, false},
		{"prefix collision", "MY_GITHUB_TOKEN=synthetic-secret", false},
		{"ordinary reference", "missing GITHUB_TOKEN environment variable", false},
		{"multiline assignment", "GITHUB_TOKEN=\nsynthetic-secret", false},
		{"lowercase assignment", "github_token=synthetic-secret", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auditCredentialAssignmentV3.MatchString(tc.input)
			if got != tc.want {
				t.Errorf("matched = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestAuditCredentialAdversarialV3(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"spaces before newline", "GITHUB_TOKEN=   \n", false},
		{"spaces before comment", "GITHUB_TOKEN:   # not set", false},
		{"YAML null", "GITHUB_TOKEN: null", false},
		{"YAML tilde", "GITHUB_TOKEN: ~", false},
		{"JSON null", `"GITHUB_TOKEN": null`, false},
		{"double quoted spaces", `GITHUB_TOKEN="   "`, false},
		{"single quoted spaces", "GITHUB_TOKEN='   '", false},
		{"unterminated double quote", `GITHUB_TOKEN="secret`, false},
		{"unterminated single quote", "GITHUB_TOKEN='secret", false},
		{"suffix collision", "GITHUB_TOKEN_EXTRA=secret", false},
		{"prefix collision", "MY_GITHUB_TOKEN=secret", false},
		{"hyphenated prefix", "MY-GITHUB_TOKEN=secret", false},
		{"assignment with tabs", "GITHUB_TOKEN\t=\tsecret", true},
		{"YAML comment after secret", "GITHUB_TOKEN: secret # note", true},
		{"JSON quoted secret", `"GITHUB_TOKEN": "secret"`, true},
		{"JSON numeric value", `"GITHUB_TOKEN": 12345`, true},
		{"ordinary environment reference", "GITHUB_TOKEN is required", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auditCredentialAssignmentV3.MatchString(tc.input)
			if got != tc.want {
				t.Errorf("matched = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestAuditWorkspaceContentV3LargeFiles(t *testing.T) {
	const size = 10 << 20

	t.Run("ordinary large source", func(t *testing.T) {
		data := []byte(strings.Repeat("package main\n", size/13))

		findings := auditWorkspaceContentV3([]capsule.WorkspaceFile{
			{Path: "large.go", Data: data},
		})

		if len(findings) != 0 {
			t.Fatalf("unexpected findings: %v", findings)
		}
	})

	t.Run("credential near end", func(t *testing.T) {
		data := []byte(strings.Repeat("a", size-100))
		data = append(
			data,
			[]byte("\nGITHUB_TOKEN=synthetic-large-file-secret\n")...,
		)

		findings := auditWorkspaceContentV3([]capsule.WorkspaceFile{
			{Path: "large.go", Data: data},
		})

		if len(findings) != 1 {
			t.Fatalf("expected one finding, got %d", len(findings))
		}

		for _, finding := range findings {
			if strings.Contains(finding, "synthetic-large-file-secret") {
				t.Fatal("finding leaked synthetic secret")
			}
		}
	})
}

func TestAuditCredentialBypassV3(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"null then secret", "GITHUB_TOKEN=null\nGH_TOKEN=synthetic-secret", true},
		{"empty then secret", "GITHUB_TOKEN=\"\"\nGH_TOKEN=synthetic-secret", true},
		{"comment then secret", "GITHUB_TOKEN: # unset\nGH_TOKEN: synthetic-secret", true},
		{"two assignments one line", "GITHUB_TOKEN=null GH_TOKEN=synthetic-secret", true},
		{"multiple lines", "ordinary text\nGH_TOKEN=synthetic-secret\nmore text", true},
		{"Unicode separator", "prefix：GITHUB_TOKEN=synthetic-secret", true},
		{"CRLF lines", "GITHUB_TOKEN=null\r\nGH_TOKEN=synthetic-secret\r\n", true},
		{"JSON two fields", "{\"GITHUB_TOKEN\":null,\"GH_TOKEN\":\"synthetic-secret\"}", true},
		{"quoted literal null", "GITHUB_TOKEN=\"null\"", true},
		{"quoted literal tilde", "GITHUB_TOKEN=\"~\"", true},
		{"comment marker in quoted value", "GITHUB_TOKEN=\"#synthetic-secret\"", true},
		{"assignment near punctuation", "(GITHUB_TOKEN=synthetic-secret)", true},
		{"suffix punctuation", "GITHUB_TOKEN=synthetic-secret;", true},
		{"multiple same variable", "GITHUB_TOKEN=null\nGITHUB_TOKEN=synthetic-secret", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auditCredentialAssignmentV3.MatchString(tc.input)
			if got != tc.want {
				t.Errorf("matched = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestAuditCredentialSequentialScanningV3(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "thousands of null assignments before secret",
			input: strings.Repeat("GITHUB_TOKEN=null\n", 5000) + "GH_TOKEN=synthetic-secret",
			want:  true,
		},
		{
			name:  "thousands of null assignments without secret",
			input: strings.Repeat("GITHUB_TOKEN=null\n", 5000),
			want:  false,
		},
		{
			name:  "adjacent JSON fields",
			input: `"GITHUB_TOKEN":null,"GH_TOKEN":"synthetic-secret"`,
			want:  true,
		},
		{
			name:  "multiple quoted empty values then secret",
			input: `GITHUB_TOKEN="" GH_TOKEN='' GITLAB_TOKEN=synthetic-secret`,
			want:  true,
		},
		{
			name:  "repeated quoted placeholders",
			input: strings.Repeat("GITHUB_TOKEN=\"\"\n", 5000),
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auditCredentialAssignmentV3.MatchString(tc.input)
			if got != tc.want {
				t.Errorf("matched = %t, want %t", got, tc.want)
			}
		})
	}
}
