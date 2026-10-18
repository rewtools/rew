package main

import (
	"regexp"
	"strings"

	"github.com/rewtools/rew/internal/capsule"
)

// auditCredentialMatcherV3 separates candidate identification
// from value validation. It is deliberately not a general-purpose
// secret scanner or YAML/JSON parser.
type auditCredentialMatcherV3 struct {
	pattern *regexp.Regexp
}

// The sole capture group contains the candidate value.
// A leading hyphen is excluded to avoid matching part of a
// longer environment variable name.
var auditCredentialAssignmentV3 = auditCredentialMatcherV3{
	pattern: regexp.MustCompile(
		`(?i)(?:^|[^A-Za-z0-9_-])` +
			`"?` +
			`(?:GITHUB_TOKEN|GH_TOKEN|GITLAB_TOKEN|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN)` +
			`"?[ \t]*[=:][ \t]*` +
			`("[^"\r\n]*"|'[^'\r\n]*'|[^\s"']+)`,
	),
}

func (m auditCredentialMatcherV3) MatchString(input string) bool {
	return m.Match([]byte(input))
}

func (m auditCredentialMatcherV3) Match(data []byte) bool {
	offset := 0

	for offset < len(data) {
		match := m.pattern.FindSubmatchIndex(data[offset:])
		if match == nil {
			return false
		}

		if len(match) < 4 || match[2] < 0 {
			return false
		}

		value := strings.TrimSpace(
			string(data[offset+match[2] : offset+match[3]]),
		)

		if auditCredentialValueV3(value) {
			return true
		}

		next := offset + match[1]
		if next <= offset {
			return false
		}
		offset = next
	}

	return false
}

func auditCredentialValueV3(value string) bool {
	quoted := false

	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			quoted = true
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}

	if value == "" {
		return false
	}

	if !quoted {
		if value == "~" ||
			strings.EqualFold(value, "null") ||
			strings.HasPrefix(value, "#") {
			return false
		}
	}

	return true
}

// auditSensitiveDataV3 reports only generic finding categories.
// It never returns credential values, filenames or source snippets.
func auditSensitiveDataV3(manifest capsule.Manifest) []string {
	var findings []string

	for _, file := range manifest.Workspace.Files {
		if isSensitiveWorkspacePathV3(file.Path) {
			findings = append(
				findings,
				"workspace path: sensitive filename",
			)
		}
	}

	for _, arg := range manifest.Execution.Argv {
		if auditCredentialAssignmentV3.MatchString(arg) {
			findings = append(
				findings,
				"command argument: credential assignment",
			)
		}
	}

	if auditCredentialAssignmentV3.MatchString(manifest.Execution.Stdout) {
		findings = append(findings, "stdout: credential assignment")
	}

	if auditCredentialAssignmentV3.MatchString(manifest.Execution.Stderr) {
		findings = append(findings, "stderr: credential assignment")
	}

	return findings
}

// auditWorkspaceContentV3 scans already-verified workspace bytes.
// It emits one generic finding per matching file.
func auditWorkspaceContentV3(files []capsule.WorkspaceFile) []string {
	var findings []string

	for _, file := range files {
		if auditCredentialAssignmentV3.Match(file.Data) {
			findings = append(
				findings,
				"workspace content: credential assignment",
			)
		}
	}

	return findings
}
