package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
)

type captureV3Fixture struct {
	workspace string
	output    string
	cacheRoot string
	goSum     []byte
}

func makeCaptureV3Fixture(t *testing.T) captureV3Fixture {
	t.Helper()
	setupCaptureV3WithoutCredentials(t)

	const modulePath = "example.com/rew/fixture"
	const moduleVersion = "v1.0.0"

	cacheRoot := t.TempDir()

	t.Cleanup(func() {
		_ = filepath.Walk(cacheRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				_ = os.Chmod(path, 0700)
			} else if info.Mode().IsRegular() {
				_ = os.Chmod(path, 0600)
			}
			return nil
		})
	})
	workspace := t.TempDir()
	output := filepath.Join(t.TempDir(), "failure.bfc")

	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOMODCACHE", cacheRoot)
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "-modcacherw")

	moduleGoMod := []byte(
		"module " + modulePath + "\n\ngo 1.26.0\n",
	)

	moduleSource := []byte(
		"package fixture\n\n" +
			"func Value() string { return \"offline-ok\" }\n",
	)

	var zipBuffer bytes.Buffer
	zw := zip.NewWriter(&zipBuffer)

	for _, entry := range []struct {
		name string
		data []byte
	}{
		{
			modulePath + "@" + moduleVersion + "/go.mod",
			moduleGoMod,
		},
		{
			modulePath + "@" + moduleVersion + "/fixture.go",
			moduleSource,
		},
	} {
		writer, err := zw.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "fixture.zip")
	if err := os.WriteFile(
		zipPath,
		zipBuffer.Bytes(),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	zipHash, err := dirhash.HashZip(
		zipPath,
		dirhash.Hash1,
	)
	if err != nil {
		t.Fatal(err)
	}

	modHash, err := dirhash.Hash1(
		[]string{"go.mod"},
		func(name string) (io.ReadCloser, error) {
			if name != "go.mod" {
				return nil, fmt.Errorf("unexpected file %s", name)
			}
			return io.NopCloser(
				bytes.NewReader(moduleGoMod),
			), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	goSum := []byte(fmt.Sprintf(
		"%s %s %s\n%s %s/go.mod %s\n",
		modulePath,
		moduleVersion,
		zipHash,
		modulePath,
		moduleVersion,
		modHash,
	))

	escapedPath, err := module.EscapePath(modulePath)
	if err != nil {
		t.Fatal(err)
	}

	escapedVersion, err := module.EscapeVersion(moduleVersion)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(
		cacheRoot,
		"cache",
		"download",
		filepath.FromSlash(escapedPath),
		"@v",
	)

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}

	for _, artifact := range []struct {
		extension string
		data      []byte
	}{
		{".zip", zipBuffer.Bytes()},
		{".mod", moduleGoMod},
		{".info", []byte(`{"Version":"v1.0.0"}`)},
	} {
		destination := filepath.Join(
			cacheDir,
			escapedVersion+artifact.extension,
		)

		if err := os.WriteFile(
			destination,
			artifact.data,
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	projectFiles := map[string][]byte{
		"go.mod": []byte(
			"module example.com/capture-v3-fixture\n\n" +
				"go 1.26.0\n\n" +
				"require " + modulePath + " " + moduleVersion + "\n",
		),
		"go.sum": goSum,
		"main.go": []byte(`package main

import "example.com/rew/fixture"

func main() {
	_ = fixture.Value()
	missingSymbol()
}
`),
	}

	for name, data := range projectFiles {
		if err := os.WriteFile(
			filepath.Join(workspace, name),
			data,
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	// Restore portable Go build flags after cache preparation.
	t.Setenv("GOFLAGS", "")

	return captureV3Fixture{
		workspace: workspace,
		output:    output,
		cacheRoot: cacheRoot,
		goSum:     goSum,
	}
}

func captureV3FixtureCommand(
	fixture captureV3Fixture,
) int {
	return captureCommandV3(
		[]string{"go", "build", "./..."},
		fixture.workspace,
		[]string{"go.mod", "go.sum", "main.go"},
		fixture.output,
	)
}

func TestCaptureV3CreatesVerifiedCapsule(t *testing.T) {
	fixture := makeCaptureV3Fixture(t)

	code := captureV3FixtureCommand(fixture)
	if code != 1 {
		t.Fatalf("capture exit=%d, want 1", code)
	}

	verified, err := capsule.ReadVerifiedV3(fixture.output)
	if err != nil {
		t.Fatalf("verify captured capsule: %v", err)
	}

	if verified.Manifest.SchemaVersion != capsule.SchemaVersionV3 {
		t.Fatal("expected schema-v3 capsule")
	}

	if verified.Manifest.Execution.ExitCode != 1 {
		t.Fatal("incorrect captured exit code")
	}

	if !strings.Contains(
		verified.Manifest.Execution.Stderr,
		"undefined: missingSymbol",
	) {
		t.Fatalf(
			"unexpected captured stderr: %q",
			verified.Manifest.Execution.Stderr,
		)
	}

	if len(verified.Manifest.GoModules) != 1 {
		t.Fatal("expected one captured Go dependency")
	}

	mod := verified.Manifest.GoModules[0]
	if mod.Path != "example.com/rew/fixture" ||
		mod.Version != "v1.0.0" {
		t.Fatalf("unexpected dependency: %+v", mod)
	}

	if len(verified.Artifacts) != 3 {
		t.Fatal("expected ZIP, MOD and INFO artifacts")
	}

	t.Log("CAPTURE V3 VERIFIED CAPSULE: PASS")
}

func TestCaptureV3RejectsInvalidGoSumWithValidCache(t *testing.T) {
	fixture := makeCaptureV3Fixture(t)

	corrupted := bytes.Replace(
		fixture.goSum,
		[]byte("h1:"),
		[]byte("h1:incorrect-"),
		1,
	)

	if err := os.WriteFile(
		filepath.Join(fixture.workspace, "go.sum"),
		corrupted,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	code := captureV3FixtureCommand(fixture)
	if code != 125 {
		t.Fatalf("capture exit=%d, want 125", code)
	}

	if _, err := os.Stat(fixture.output); !os.IsNotExist(err) {
		t.Fatalf("invalid go.sum created archive: %v", err)
	}

	t.Log("INVALID GO.SUM REJECTED WITH CACHE PRESENT: PASS")
}

func TestCaptureV3DoesNotOverwriteArchive(t *testing.T) {
	fixture := makeCaptureV3Fixture(t)

	original := []byte("EXISTING_ARCHIVE_DO_NOT_OVERWRITE\n")

	if err := os.WriteFile(
		fixture.output,
		original,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	code := captureV3FixtureCommand(fixture)
	if code != 125 {
		t.Fatalf("capture exit=%d, want 125", code)
	}

	after, err := os.ReadFile(fixture.output)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(after, original) {
		t.Fatal("existing archive was overwritten")
	}

	t.Log("EXISTING ARCHIVE PRESERVED: PASS")
}
