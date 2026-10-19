package capsule

import "testing"

func TestParseGoModuleGraphV4(t *testing.T) {
	input := []byte(`
{
  "Path": "example.com/main-app",
  "Main": true,
  "Dir": "/original/app",
  "GoMod": "/original/app/go.mod",
  "GoVersion": "1.25"
}
{
  "Path": "example.com/local-lib",
  "Version": "v0.0.0",
  "Replace": {
    "Path": "../lib",
    "Dir": "/original/lib",
    "GoMod": "/original/lib/go.mod",
    "GoVersion": "1.25"
  },
  "Dir": "/original/lib",
  "GoMod": "/original/lib/go.mod"
}
`)

	modules, err := ParseGoModuleGraphV4(input)
	if err != nil {
		t.Fatal(err)
	}

	if len(modules) != 1 {
		t.Fatalf("modules = %d, want 1", len(modules))
	}

	mod := modules[0]

	if mod.Path != "example.com/local-lib" ||
		mod.Version != "v0.0.0" {
		t.Fatalf("unexpected module identity: %+v", mod)
	}

	if mod.Replace == nil || mod.Replace.Path != "../lib" {
		t.Fatalf("local replacement missing: %+v", mod.Replace)
	}

	// Absolute capture-machine paths must never be recorded
	// as portable replacement metadata.
	if mod.Replace.SnapshotPath != "" {
		t.Fatalf("parser must not invent snapshot path: %+v", mod.Replace)
	}
}

func TestParseGoModuleGraphV4RejectsInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "missing main module",
			input: `{"Path":"example.com/lib","Version":"v0.0.0"}`,
		},
		{
			name: "duplicate main module",
			input: `{"Path":"example.com/app","Main":true}
{"Path":"example.com/app","Main":true}`,
		},
		{
			name: "duplicate module",
			input: `{"Path":"example.com/app","Main":true}
{"Path":"example.com/lib","Version":"v0.0.0"}
{"Path":"example.com/lib","Version":"v0.0.0"}`,
		},
		{
			name: "invalid replacement path",
			input: `{"Path":"example.com/app","Main":true}
{"Path":"example.com/lib","Version":"v0.0.0",
"Replace":{"Path":"/tmp/lib"}}`,
		},
		{
			name: "replacement with version",
			input: `{"Path":"example.com/app","Main":true}
{"Path":"example.com/lib","Version":"v0.0.0",
"Replace":{"Path":"example.com/other","Version":"v1.0.0"}}`,
		},
		{
			name: "module graph error",
			input: `{"Path":"example.com/app","Main":true}
{"Path":"example.com/lib","Version":"v0.0.0",
"Error":{"Err":"missing module"}}`,
		},
		{
			name: "invalid module version",
			input: `{"Path":"example.com/app","Main":true}
{"Path":"example.com/lib","Version":"not-a-version"}`,
		},
		{
			name:  "unexpected JSON field",
			input: `{"Path":"example.com/app","Main":true,"Unexpected":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseGoModuleGraphV4([]byte(tt.input)); err == nil {
				t.Fatal("invalid graph unexpectedly accepted")
			}
		})
	}
}

func TestParseGoModuleGraphV4RejectsUnsafeReplacementMetadata(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
	}{
		{"unknown field", `{"Path":"../lib","Unexpected":true}`},
		{"unknown nested field", `{"Path":"../lib","Error":{"Err":"hidden"}}`},
		{"dot interior", `{"Path":".././lib"}`},
		{"parent interior", `{"Path":"../other/../lib"}`},
		{"empty component", `{"Path":"..//lib"}`},
		{"NUL path", `{"Path":"../lib\u0000"}`},
		{"absolute path", `{"Path":"/tmp/lib"}`},
		{"remote module replacement", `{"Path":"example.com/other","Version":"v1.0.0"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(
				`{"Path":"example.com/app","Main":true}` + "\n" +
					`{"Path":"example.com/lib","Version":"v0.0.0","Replace":` +
					tt.replacement + `}`,
			)

			if _, err := ParseGoModuleGraphV4(input); err == nil {
				t.Fatal("unsafe replacement metadata accepted")
			}
		})
	}
}
