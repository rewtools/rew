package docker

import (
	"bytes"
	"context"
	"testing"
)

func TestMixedV4ExecutorRejectsInvalidExternalModule(t *testing.T) {
	cases := []struct {
		name     string
		external string
		version  string
	}{
		{
			name:     "invalid external path",
			external: "../outside",
			version:  "v1.0.0",
		},
		{
			name:     "invalid external version",
			external: "example.com/external",
			version:  "not-a-version",
		},
		{
			name:     "external equals main",
			external: "example.com/app",
			version:  "v1.0.0",
		},
		{
			name:     "external equals local",
			external: "example.com/local-lib",
			version:  "v1.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			_, err := RunWithSDKRuntimeSnapshotExecReadersMixedV4(
				context.Background(),
				bytes.NewReader([]byte("workspace")),
				bytes.NewReader([]byte("modules")),
				[]string{"go", "build", "./..."},
				"example.com/app",
				"example.com/local-lib",
				tc.external,
				tc.version,
				256<<20,
				"10001:10001",
				&stdout,
				&stderr,
				&RuntimeSpec{
					Kind:    "go",
					Version: "1.26.6",
				},
			)
			if err == nil {
				t.Fatal("invalid mixed V4 module identity accepted")
			}
		})
	}
}
