package docker

import (
	"fmt"
	"testing"
)

func TestParseDockerGoModuleGraphV4(t *testing.T) {
	const mainModule = "example.com/app"
	const localModule = "example.com/local-lib"

	tests := []struct {
		name        string
		replacement string
		wantErr     bool
	}{
		{
			name:        "valid sibling replacement",
			replacement: `{"Path":"../lib","Dir":"/workspace/lib","GoMod":"/workspace/lib/go.mod"}`,
		},
		{
			name:        "remote replacement",
			replacement: `{"Path":"example.com/remote","Version":"v1.2.3"}`,
			wantErr:     true,
		},
		{
			name:        "wrong sibling directory",
			replacement: `{"Path":"../lib","Dir":"/workspace/other","GoMod":"/workspace/lib/go.mod"}`,
			wantErr:     true,
		},
		{
			name:        "host path",
			replacement: `{"Path":"../lib","Dir":"/tmp/lib","GoMod":"/workspace/lib/go.mod"}`,
			wantErr:     true,
		},
		{
			name:        "missing replacement",
			replacement: "null",
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph := fmt.Sprintf(
				`{"Path":%q,"Main":true,"Dir":"/workspace/app","GoMod":"/workspace/app/go.mod"}`+"\n"+
					`{"Path":%q,"Version":"v0.0.0","Dir":"/workspace/lib","GoMod":"/workspace/lib/go.mod","Replace":%s}`+"\n",
				mainModule,
				localModule,
				tc.replacement,
			)

			err := verifyDockerGoModuleGraphV4(
				[]byte(graph),
				mainModule,
				localModule,
				"/workspace/lib",
			)

			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"graph verification error=%v, wantErr=%v",
					err, tc.wantErr,
				)
			}
		})
	}
}

func TestParseDockerGoModuleGraphV4RejectsExtraModules(t *testing.T) {
	graph := []byte(
		`{"Path":"example.com/app","Main":true,"Dir":"/workspace/app","GoMod":"/workspace/app/go.mod"}` + "\n" +
			`{"Path":"example.com/local-lib","Version":"v0.0.0","Dir":"/workspace/lib","GoMod":"/workspace/lib/go.mod",` +
			`"Replace":{"Path":"../lib","Dir":"/workspace/lib","GoMod":"/workspace/lib/go.mod"}}` + "\n" +
			`{"Path":"example.com/unexpected","Version":"v1.0.0"}` + "\n",
	)

	if err := verifyDockerGoModuleGraphV4(
		graph,
		"example.com/app",
		"example.com/local-lib",
		"/workspace/lib",
	); err == nil {
		t.Fatal("unexpected external module accepted")
	}
}

func TestParseDockerGoModuleGraphV4RejectsInvalidJSON(t *testing.T) {
	if err := verifyDockerGoModuleGraphV4(
		[]byte(`{invalid`),
		"example.com/app",
		"example.com/local-lib",
		"/workspace/lib",
	); err == nil {
		t.Fatal("invalid graph JSON accepted")
	}
}
