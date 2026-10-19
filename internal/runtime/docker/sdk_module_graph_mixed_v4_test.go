package docker

import (
	"strings"
	"testing"
)

func validMixedDockerGraphV4() string {
	return `{"Path":"example.com/app","Main":true,"Dir":"/workspace/app","GoMod":"/workspace/app/go.mod"}` + "\n" +
		`{"Path":"example.com/external","Version":"v1.0.0"}` + "\n" +
		`{"Path":"example.com/local-lib","Version":"v0.0.0","Dir":"/workspace/lib","GoMod":"/workspace/lib/go.mod","Replace":{"Path":"../lib","Dir":"/workspace/lib","GoMod":"/workspace/lib/go.mod"}}` + "\n"
}

func TestVerifyDockerGoModuleGraphMixedV4(t *testing.T) {
	const mainModule = "example.com/app"
	const localModule = "example.com/local-lib"
	const externalModule = "example.com/external"
	const externalVersion = "v1.0.0"

	valid := validMixedDockerGraphV4()

	cases := []struct {
		name    string
		graph   string
		wantErr bool
	}{
		{
			name:  "valid mixed graph",
			graph: valid,
		},
		{
			name: "wrong external version",
			graph: strings.Replace(
				valid, `"Version":"v1.0.0"`,
				`"Version":"v1.0.1"`, 1,
			),
			wantErr: true,
		},
		{
			name: "unexpected fourth module",
			graph: valid +
				`{"Path":"example.com/extra","Version":"v1.0.0"}` + "\n",
			wantErr: true,
		},
		{
			name: "missing external module",
			graph: strings.Replace(
				valid,
				`{"Path":"example.com/external","Version":"v1.0.0"}`+"\n",
				"", 1,
			),
			wantErr: true,
		},
		{
			name: "external module replacement",
			graph: strings.Replace(
				valid,
				`{"Path":"example.com/external","Version":"v1.0.0"}`,
				`{"Path":"example.com/external","Version":"v1.0.0","Replace":{"Path":"../other"}}`,
				1,
			),
			wantErr: true,
		},
		{
			name: "wrong local replacement",
			graph: strings.Replace(
				valid,
				`"Path":"../lib"`,
				`"Path":"../other"`, 1,
			),
			wantErr: true,
		},
		{
			name: "duplicate external module",
			graph: valid +
				`{"Path":"example.com/external","Version":"v1.0.0"}` + "\n",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			graph:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyDockerGoModuleGraphMixedV4(
				[]byte(tc.graph),
				mainModule,
				localModule,
				"/workspace/lib",
				externalModule,
				externalVersion,
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

func TestVerifyDockerGoModuleGraphMixedV4RejectsDuplicateJSONKeys(
	t *testing.T,
) {
	valid := validMixedDockerGraphV4()

	cases := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "duplicate main path",
			old:  `"Path":"example.com/app","Main":true`,
			new:  `"Path":"example.com/evil","Path":"example.com/app","Main":true`,
		},
		{
			name: "duplicate local replacement path",
			old:  `"Replace":{"Path":"../lib","Dir":"/workspace/lib"`,
			new:  `"Replace":{"Path":"../other","Path":"../lib","Dir":"/workspace/lib"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(valid, tc.old) != 1 {
				t.Fatal("test fixture structure changed")
			}

			graph := strings.Replace(valid, tc.old, tc.new, 1)

			err := verifyDockerGoModuleGraphMixedV4(
				[]byte(graph),
				"example.com/app",
				"example.com/local-lib",
				"/workspace/lib",
				"example.com/external",
				"v1.0.0",
			)

			if err == nil {
				t.Fatal(
					"SECURITY FAILURE: duplicate JSON key accepted",
				)
			}
		})
	}
}

func TestMixedV4GraphSecurityBoundaries(t *testing.T) {
	valid := []byte(validMixedDockerGraphV4())

	verify := func(
		data []byte,
		main, local, directory, external, version string,
	) error {
		return verifyDockerGoModuleGraphMixedV4(
			data, main, local, directory, external, version,
		)
	}

	t.Run("oversized JSON", func(t *testing.T) {
		oversized := make([]byte, (1<<20)+1)
		if err := verify(
			oversized,
			"example.com/app",
			"example.com/local-lib",
			"/workspace/lib",
			"example.com/external",
			"v1.0.0",
		); err == nil {
			t.Fatal("oversized module graph accepted")
		}
	})

	t.Run("empty JSON", func(t *testing.T) {
		if err := verify(
			nil,
			"example.com/app",
			"example.com/local-lib",
			"/workspace/lib",
			"example.com/external",
			"v1.0.0",
		); err == nil {
			t.Fatal("empty module graph accepted")
		}
	})

	t.Run("deep nesting", func(t *testing.T) {
		deep := []byte(
			`{"nested":` +
				strings.Repeat("[", 33) +
				`0` +
				strings.Repeat("]", 33) +
				`}`,
		)
		if err := rejectDuplicateMixedV4JSONKeys(deep); err == nil {
			t.Fatal("excessively nested JSON accepted")
		}
	})

	t.Run("nested JSON within depth limit", func(t *testing.T) {
		nested := []byte(`{"nested":[{"key":"value"}]}`)
		if err := rejectDuplicateMixedV4JSONKeys(nested); err != nil {
			t.Fatalf("valid nested JSON rejected: %v", err)
		}
	})

	t.Run("truncated JSON", func(t *testing.T) {
		if err := rejectDuplicateMixedV4JSONKeys(
			[]byte(`{"Path":"example.com/app"`),
		); err == nil {
			t.Fatal("truncated JSON accepted")
		}
	})

	cases := []struct {
		name      string
		main      string
		local     string
		directory string
		external  string
		version   string
	}{
		{
			name:      "invalid main path",
			main:      "INVALID MAIN",
			local:     "example.com/local-lib",
			directory: "/workspace/lib",
			external:  "example.com/external",
			version:   "v1.0.0",
		},
		{
			name:      "invalid local path",
			main:      "example.com/app",
			local:     "INVALID LOCAL",
			directory: "/workspace/lib",
			external:  "example.com/external",
			version:   "v1.0.0",
		},
		{
			name:      "invalid external path",
			main:      "example.com/app",
			local:     "example.com/local-lib",
			directory: "/workspace/lib",
			external:  "INVALID EXTERNAL",
			version:   "v1.0.0",
		},
		{
			name:      "invalid external version",
			main:      "example.com/app",
			local:     "example.com/local-lib",
			directory: "/workspace/lib",
			external:  "example.com/external",
			version:   "not-a-version",
		},
		{
			name:      "duplicate main and local identities",
			main:      "example.com/local-lib",
			local:     "example.com/local-lib",
			directory: "/workspace/lib",
			external:  "example.com/external",
			version:   "v1.0.0",
		},
		{
			name:      "duplicate local and external identities",
			main:      "example.com/app",
			local:     "example.com/external",
			directory: "/workspace/lib",
			external:  "example.com/external",
			version:   "v1.0.0",
		},
		{
			name:      "unexpected local directory",
			main:      "example.com/app",
			local:     "example.com/local-lib",
			directory: "/workspace/other",
			external:  "example.com/external",
			version:   "v1.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := verify(
				valid,
				tc.main,
				tc.local,
				tc.directory,
				tc.external,
				tc.version,
			); err == nil {
				t.Fatal("invalid expected graph identity accepted")
			}
		})
	}
}
