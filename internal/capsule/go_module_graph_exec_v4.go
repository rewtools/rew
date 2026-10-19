package capsule

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ListGoModuleGraphV4WithExecutable obtains the selected module graph
// from the main Go module and parses its effective local replacements.
//
// Like the v3 runner, it uses offline, bounded Go execution.
// This invokes the Go toolchain and is not a security sandbox.
func ListGoModuleGraphV4WithExecutable(
	workspaceRoot string,
	moduleCacheRoot string,
	goExecutable string,
) ([]GoModuleIdentityV4, error) {
	if workspaceRoot == "" ||
		moduleCacheRoot == "" ||
		!filepath.IsAbs(goExecutable) {
		return nil, fmt.Errorf(
			"capsule: invalid v4 Go module graph input",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		goModuleGraphTimeoutV3,
	)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		goExecutable,
		"list", "-m", "-json", "all",
	)
	cmd.Dir = workspaceRoot

	env := make([]string, 0, len(os.Environ())+9)

	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")

		switch key {
		case "GOOS", "GOARCH", "CGO_ENABLED",
			"GOFLAGS", "GOWORK", "GOTOOLCHAIN",
			"GOPROXY", "GOSUMDB", "GOMODCACHE":
			continue
		}

		env = append(env, value)
	}

	cmd.Env = append(
		env,
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
		"GOFLAGS=-mod=readonly",
		"GOWORK=off",
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOMODCACHE="+moduleCacheRoot,
	)

	stdout := limitedOutputV3{
		limit: goModuleGraphMaxOutput,
	}
	stderr := limitedOutputV3{
		limit: 64 << 10,
	}

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf(
				"capsule: v4 Go module graph timed out: %w",
				ctx.Err(),
			)
		}

		return nil, fmt.Errorf(
			"capsule: offline v4 Go module graph failed: %w: %s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}

	if len(stdout.Bytes()) > goModuleGraphMaxOutput {
		return nil, fmt.Errorf(
			"capsule: v4 Go module graph exceeds output limit",
		)
	}

	return ParseGoModuleGraphV4(stdout.Bytes())
}
