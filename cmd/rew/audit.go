package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rewtools/rew/internal/capsule"
)

const auditUsage = "Usage: rew audit <capsule.bfc>"

// runAudit verifies a schema-v3 capsule without running its command.
// It also performs limited sensitive-data checks on verified contents.
// It does not establish whether the capsule is safe to share.
func runAudit(args []string) int {
	if len(args) != 1 ||
		args[0] == "" ||
		strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, auditUsage)
		return 2
	}

	verified, err := capsule.ReadVerifiedToStoreV3(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "rew: audit: capsule verification failed")
		return 125
	}

	manifest := verified.Manifest
	fileCount := len(verified.Files)
	moduleCount := len(manifest.GoModules)
	findings := auditSensitiveDataV3(manifest)
	findings = append(findings, auditWorkspaceContentV3(verified.Files)...)

	if err := verified.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "rew: audit: temporary artifact cleanup failed")
		return 125
	}

	fmt.Println("Capsule integrity: verified")
	fmt.Printf("Schema version: %d\n", manifest.SchemaVersion)
	fmt.Printf("Workspace files: %d\n", fileCount)
	fmt.Printf("Go modules: %d\n", moduleCount)
	if len(findings) != 0 {
		fmt.Printf("Sensitive data findings: %d\\n", len(findings))
		for _, finding := range findings {
			fmt.Printf("Finding: %s\\n", finding)
		}
		fmt.Println("WARNING: Potentially sensitive data detected. Do not share without review.")
		return 3
	}

	fmt.Println("Sensitive data findings: 0")
	fmt.Println("Scope: sensitive workspace filenames and known credential assignments in argv/stdout/stderr and workspace contents.")
	fmt.Println("WARNING: This limited audit does not establish that the capsule is safe to share.")
	return 0
}
