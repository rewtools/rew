package docker

import "testing"

func TestDockerGoModuleGraphV4RealGoOutput(t *testing.T) {
	// Representative output of:
	// GOPROXY=off GOWORK=off go list -m -json all
	// from Go 1.26.6 with a sibling local replacement.
	const graph = `{
		"Path": "example.com/app",
		"Main": true,
		"Dir": "/workspace/app",
		"GoMod": "/workspace/app/go.mod",
		"GoVersion": "1.25"
	}
	{
		"Path": "example.com/local-lib",
		"Version": "v0.0.0",
		"Replace": {
			"Path": "../lib",
			"Dir": "/workspace/lib",
			"GoMod": "/workspace/lib/go.mod",
			"GoVersion": "1.25"
		},
		"Dir": "/workspace/lib",
		"GoMod": "/workspace/lib/go.mod",
		"GoVersion": "1.25"
	}`

	if err := verifyDockerGoModuleGraphV4(
		[]byte(graph),
		"example.com/app",
		"example.com/local-lib",
		"/workspace/lib",
	); err != nil {
		t.Fatalf("valid real Go module graph rejected: %v", err)
	}
}
