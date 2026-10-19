package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/execution"
)

func TestCaptureV4MixedPreflightAndArtifactIntegrity(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")
	proxy := filepath.Join(parent, "proxy")
	cache := filepath.Join(parent, "cache")
	output := filepath.Join(parent, "failure.rew")

	t.Cleanup(func() {
		// Go's extracted module cache is intentionally read-only.
		// Restore directory and file write permissions before
		// testing.TempDir performs its recursive cleanup.
		err := filepath.Walk(cache, func(
			name string,
			info os.FileInfo,
			walkErr error,
		) error {
			if os.IsNotExist(walkErr) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if info.Mode().IsRegular() || info.IsDir() {
				return os.Chmod(name, info.Mode().Perm()|0700)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Errorf("restore module cache permissions: %v", err)
		}
	})

	write := func(name string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"), []byte(`module example.com/app

go 1.25

require (
	example.com/local-lib v0.0.0
	example.com/external v1.0.0
)

replace example.com/local-lib => ../lib
`))

	write(filepath.Join(app, "main.go"), []byte(`package main

import (
	_ "example.com/local-lib"
	_ "example.com/external"
)

func main() {}
`))

	write(filepath.Join(lib, "go.mod"), []byte(
		"module example.com/local-lib\n\ngo 1.25\n",
	))
	write(filepath.Join(lib, "lib.go"), []byte(
		"package lib\n\nfunc Value() {}\n",
	))

	const externalModule = "example.com/external"
	const version = "v1.0.0"

	externalMod := []byte(
		"module example.com/external\n\ngo 1.25\n",
	)

	proxyVersionDir := filepath.Join(
		proxy, "example.com", "external", "@v",
	)

	write(
		filepath.Join(proxyVersionDir, version+".mod"),
		externalMod,
	)
	write(
		filepath.Join(proxyVersionDir, version+".info"),
		[]byte(`{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`),
	)
	write(
		filepath.Join(proxyVersionDir, "list"),
		[]byte(version+"\n"),
	)

	var zipData bytes.Buffer
	zipWriter := zip.NewWriter(&zipData)

	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"go.mod", externalMod},
		{"external.go", []byte("package external\n\nfunc Value() {}\n")},
	} {
		name := externalModule + "@" + version + "/" + entry.name
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	write(
		filepath.Join(proxyVersionDir, version+".zip"),
		zipData.Bytes(),
	)

	goExecutable, err := capsule.ResolveGoExecutable()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOMODCACHE", cache)
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy))

	download := exec.Command(
		goExecutable, "mod", "download", externalModule+"@"+version,
	)
	download.Dir = app

	if data, err := download.CombinedOutput(); err != nil {
		t.Fatalf("populate cache from local proxy: %v\n%s", err, data)
	}

	t.Setenv("GOPROXY", "off")

	selected, err := capsule.ListGoModuleGraphV4WithExecutable(
		app, cache, goExecutable,
	)
	if err != nil {
		t.Fatalf("offline mixed graph discovery: %v", err)
	}

	localIndex, externalIndex, err :=
		classifyCaptureModuleGraphV4(selected, "../lib")
	if err != nil {
		t.Fatalf("valid mixed graph rejected: %v", err)
	}

	if localIndex < 0 || externalIndex < 0 ||
		localIndex == externalIndex {
		t.Fatalf(
			"expected local and external modules, got %d/%d",
			localIndex, externalIndex,
		)
	}

	if selected[localIndex].Path != "example.com/local-lib" ||
		selected[externalIndex].Path != externalModule ||
		selected[externalIndex].Version != version {
		t.Fatalf("unexpected mixed module identities: %+v", selected)
	}

	executorCalls := 0

	spy := func(
		argv []string,
		executable string,
		dir string,
		env []string,
		stdout, stderr io.Writer,
	) (execution.Result, error) {
		executorCalls++
		return execution.Result{}, nil
	}

	code := captureCommandV4WithExecutor(
		[]string{"go", "build", "./..."},
		app,
		output,
		spy,
	)

	// The injected executor reports a successful build.
	// A successful capture must not publish a failure capsule.
	if code != 0 {
		t.Fatalf("mixed successful build exit=%d, want 0", code)
	}
	if executorCalls != 1 {
		t.Fatalf("executor calls=%d, want 1", executorCalls)
	}

	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("successful build unexpectedly published capsule: %v", err)
	}

	t.Run("workspace mutation during execution", func(t *testing.T) {
		sourcePath := filepath.Join(lib, "lib.go")

		original, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(sourcePath)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if err := os.WriteFile(
				sourcePath, original, info.Mode().Perm(),
			); err != nil {
				t.Errorf("restore mutated source: %v", err)
			}
		})

		mutationCalls := 0
		mutatingExecutor := func(
			argv []string,
			executable string,
			dir string,
			env []string,
			stdout, stderr io.Writer,
		) (execution.Result, error) {
			mutationCalls++

			mutated := append(
				append([]byte(nil), original...),
				[]byte("\n// changed during build\n")...,
			)

			if err := os.WriteFile(
				sourcePath, mutated, info.Mode().Perm(),
			); err != nil {
				return execution.Result{}, err
			}

			return execution.Result{
				Argv:     argv,
				ExitCode: 1,
				Stderr:   "synthetic failed build",
			}, nil
		}

		mutationOutput := filepath.Join(
			parent, "mutated-workspace.rew",
		)

		code, diagnostic := captureV4StderrForTest(t, func() int {
			return captureCommandV4WithExecutor(
				[]string{"go", "build", "./..."},
				app,
				mutationOutput,
				mutatingExecutor,
			)
		})

		if mutationCalls != 1 {
			t.Fatalf(
				"executor calls=%d, want 1",
				mutationCalls,
			)
		}

		if code != 125 {
			t.Fatalf(
				"mutated mixed capture exit=%d, want 125: %s",
				code, diagnostic,
			)
		}

		if !strings.Contains(
			diagnostic, "workspace changed after execution",
		) {
			t.Fatalf(
				"unexpected mutation rejection: %s",
				diagnostic,
			)
		}

		if _, err := os.Lstat(mutationOutput); !os.IsNotExist(err) {
			t.Fatalf(
				"capsule published despite source mutation: %v",
				err,
			)
		}

		t.Log("mixed source mutation rejected before publication")
	})

	t.Run("go.sum mutation during execution", func(t *testing.T) {
		sumPath := filepath.Join(app, "go.sum")

		original, err := os.ReadFile(sumPath)
		if err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(sumPath)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if err := os.WriteFile(
				sumPath, original, info.Mode().Perm(),
			); err != nil {
				t.Errorf("restore go.sum: %v", err)
			}
		})

		calls := 0
		mutatingExecutor := func(
			argv []string,
			executable string,
			dir string,
			env []string,
			stdout, stderr io.Writer,
		) (execution.Result, error) {
			calls++

			// Append only a blank line: preserve existing module
			// checksums, but change the pinned file's bytes.
			mutated := append(
				append([]byte(nil), original...),
				'\n',
			)

			if err := os.WriteFile(
				sumPath, mutated, info.Mode().Perm(),
			); err != nil {
				return execution.Result{}, err
			}

			return execution.Result{
				Argv:     argv,
				ExitCode: 1,
				Stderr:   "synthetic failed build",
			}, nil
		}

		mutationOutput := filepath.Join(
			parent, "mutated-go-sum.rew",
		)

		code, diagnostic := captureV4StderrForTest(t, func() int {
			return captureCommandV4WithExecutor(
				[]string{"go", "build", "./..."},
				app,
				mutationOutput,
				mutatingExecutor,
			)
		})

		if calls != 1 {
			t.Fatalf("executor calls=%d, want 1", calls)
		}

		if code != 125 {
			t.Fatalf(
				"mutated go.sum capture exit=%d, want 125: %s",
				code, diagnostic,
			)
		}

		if !strings.Contains(
			diagnostic, "workspace changed after execution",
		) {
			t.Fatalf(
				"unexpected go.sum mutation rejection: %s",
				diagnostic,
			)
		}

		if _, err := os.Lstat(mutationOutput); !os.IsNotExist(err) {
			t.Fatalf(
				"capsule published despite go.sum mutation: %v",
				err,
			)
		}

		t.Log("mixed pinned go.sum mutation rejected before publication")
	})

	t.Run("existing output preserved", func(t *testing.T) {
		occupiedOutput := filepath.Join(
			parent, "preexisting-output.rew",
		)
		sentinel := []byte(
			"REW_TEST_SENTINEL_EXISTING_FILE_DO_NOT_OVERWRITE\n",
		)

		if err := os.WriteFile(
			occupiedOutput, sentinel, 0600,
		); err != nil {
			t.Fatal(err)
		}

		calls := 0
		executor := func(
			argv []string,
			executable string,
			dir string,
			env []string,
			stdout, stderr io.Writer,
		) (execution.Result, error) {
			calls++
			return execution.Result{
				Argv:     argv,
				ExitCode: 1,
				Stderr:   "synthetic failed Go build",
			}, nil
		}

		code, diagnostic := captureV4StderrForTest(t, func() int {
			return captureCommandV4WithExecutor(
				[]string{"go", "build", "./..."},
				app,
				occupiedOutput,
				executor,
			)
		})

		if code != 125 {
			t.Fatalf(
				"occupied output exit=%d, want 125: %s",
				code, diagnostic,
			)
		}

		// Output may be rejected during initial validation or
		// immediately before exclusive creation. Both are safe.
		if calls > 1 {
			t.Fatalf("executor called %d times", calls)
		}

		if !strings.Contains(diagnostic, "output") {
			t.Fatalf(
				"unexpected output rejection: %s",
				diagnostic,
			)
		}

		got, err := os.ReadFile(occupiedOutput)
		if err != nil {
			t.Fatalf("existing output was removed: %v", err)
		}
		if !bytes.Equal(got, sentinel) {
			t.Fatalf(
				"existing output was modified: %q",
				got,
			)
		}

		info, err := os.Lstat(occupiedOutput)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf(
				"existing output type changed: %v",
				info.Mode(),
			)
		}

		t.Logf(
			"existing output preserved; executor calls=%d",
			calls,
		)
	})

	t.Run("output symlink preserved", func(t *testing.T) {
		target := filepath.Join(parent, "symlink-target.txt")
		outputLink := filepath.Join(parent, "symlink-output.rew")

		sentinel := []byte(
			"REW_SYMLINK_TARGET_MUST_REMAIN_UNCHANGED\n",
		)
		if err := os.WriteFile(target, sentinel, 0600); err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(target, outputLink); err != nil {
			t.Fatal(err)
		}

		calls := 0
		spy := func(
			argv []string,
			executable string,
			dir string,
			env []string,
			stdout, stderr io.Writer,
		) (execution.Result, error) {
			calls++
			return execution.Result{
				Argv:     argv,
				ExitCode: 1,
				Stderr:   "synthetic failed Go build",
			}, nil
		}

		code, diagnostic := captureV4StderrForTest(t, func() int {
			return captureCommandV4WithExecutor(
				[]string{"go", "build", "./..."},
				app,
				outputLink,
				spy,
			)
		})

		if code != 125 {
			t.Fatalf(
				"symlink output exit=%d, want 125: %s",
				code, diagnostic,
			)
		}

		if calls != 0 {
			t.Fatalf(
				"executor called for symlink output: %d",
				calls,
			)
		}

		if !strings.Contains(diagnostic, "output") {
			t.Fatalf(
				"unexpected symlink rejection: %s",
				diagnostic,
			)
		}

		linkInfo, err := os.Lstat(outputLink)
		if err != nil {
			t.Fatalf("output symlink removed: %v", err)
		}
		if linkInfo.Mode()&os.ModeSymlink == 0 {
			t.Fatalf(
				"output symlink replaced: %v",
				linkInfo.Mode(),
			)
		}

		gotLink, err := os.Readlink(outputLink)
		if err != nil {
			t.Fatal(err)
		}
		if gotLink != target {
			t.Fatalf(
				"output symlink target changed: %q",
				gotLink,
			)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("symlink target removed: %v", err)
		}
		if !bytes.Equal(got, sentinel) {
			t.Fatalf(
				"symlink target modified: %q",
				got,
			)
		}

		t.Log("output symlink and target preserved; executor not called")
	})

	graphAgain, err := capsule.ListGoModuleGraphV4WithExecutable(
		app, cache, goExecutable,
	)
	if err != nil {
		t.Fatalf("offline graph unexpectedly unavailable: %v", err)
	}
	if len(graphAgain) != len(selected) {
		t.Fatalf("graph changed during rejection: %d -> %d",
			len(selected), len(graphAgain))
	}
	for i := range selected {
		if selected[i].Path != graphAgain[i].Path ||
			selected[i].Version != graphAgain[i].Version {
			t.Fatal("module graph changed during rejection")
		}
	}

	sumData, err := os.ReadFile(filepath.Join(app, "go.sum"))
	if err != nil {
		t.Fatalf("offline external module missing go.sum: %v", err)
	}
	if !strings.Contains(string(sumData), externalModule+" "+version) {
		t.Fatal("go.sum does not contain external dependency")
	}

	// Mixed V4 must pin go.sum alongside its source files.
	// Source discovery alone is not responsible for including it.
	mainPaths, localPaths, err :=
		capsule.DiscoverLocalGoSourcesV4WithExecutable(
			app,
			"../lib",
			cache,
			goExecutable,
			capsule.GoBuildEnvironment{
				GOOS:       "linux",
				GOARCH:     "amd64",
				CGOEnabled: "0",
			},
		)
	if err != nil {
		t.Fatalf("discover mixed V4 source files: %v", err)
	}

	for _, name := range mainPaths {
		if name == "go.sum" {
			t.Fatal("go.sum unexpectedly included by source discovery")
		}
	}

	mainPaths = append(mainPaths, "go.sum")
	sort.Strings(mainPaths)

	session, err := capsule.OpenPinnedLocalModuleSessionV4(
		app, "../lib",
	)
	if err != nil {
		t.Fatalf("open pinned mixed workspace: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close pinned mixed workspace: %v", err)
		}
	}()

	pinnedFiles, err := session.Read(mainPaths, localPaths)
	if err != nil {
		t.Fatalf("read pinned mixed workspace: %v", err)
	}

	foundPinnedSum := false
	for _, file := range pinnedFiles {
		if file.Path == "app/go.sum" {
			if foundPinnedSum {
				t.Fatal("duplicate pinned app/go.sum")
			}
			foundPinnedSum = true
			if !bytes.Equal(file.Data, sumData) {
				t.Fatal("pinned go.sum differs from verified bytes")
			}
		}
	}
	if !foundPinnedSum {
		t.Fatal("pinned workspace missing app/go.sum")
	}

	if err := session.Verify(
		mainPaths, localPaths, pinnedFiles,
	); err != nil {
		t.Fatalf("verify pinned mixed workspace: %v", err)
	}

	t.Log("app/go.sum included and verified through pinned workspace session")

	// Exercise the production V3 artifact reader using the exact
	// external identity discovered from the real V4 Go graph.
	identities, err := captureExternalModuleIdentitiesV4(
		selected, "../lib",
	)
	if err != nil {
		t.Fatalf("adapt external identity: %v", err)
	}
	if len(identities) != 1 {
		t.Fatalf("external identity count=%d, want 1", len(identities))
	}

	store, err := capsule.NewArtifactStoreV3(
		capsule.MaxGoModuleTotalSizeV3,
	)
	if err != nil {
		t.Fatalf("create private artifact store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("close artifact store: %v", err)
		}
	}()

	artifacts, err := capsule.ReadGoModuleArtifactSetToStoreV3(
		identities, sumData, cache, store,
	)
	if err != nil {
		t.Fatalf("capture verified external artifacts: %v", err)
	}

	if len(artifacts) != 1 ||
		artifacts[0].Path != externalModule ||
		artifacts[0].Version != version ||
		len(artifacts[0].Artifacts) != 3 {
		t.Fatalf("unexpected external artifact metadata: %+v", artifacts)
	}

	// SnapshotPath is assigned by capture after graph validation.
	// Work on a copy to preserve the observed graph.
	verifiedSelected := append(
		[]capsule.GoModuleIdentityV4(nil), selected...,
	)
	local := verifiedSelected[localIndex]
	replacement := *local.Replace
	replacement.SnapshotPath = "lib"
	local.Replace = &replacement
	verifiedSelected[localIndex] = local

	if err := capsule.VerifyMixedGoModuleArtifactsV4(
		verifiedSelected, artifacts, sumData, store,
	); err != nil {
		t.Fatalf("mixed v4 artifact verification: %v", err)
	}

	t.Log("mixed graph resolved offline; executor not called; capsule not published")
	t.Log("external ZIP/MOD/INFO captured and verified in private disk-backed store")

	// Each negative uses a new store, so a partially captured
	// artifact set cannot contaminate the next case.
	for _, tc := range []struct {
		name string
		ext  string
	}{
		{"corrupted ZIP", ".zip"},
		{"corrupted MOD", ".mod"},
		{"corrupted INFO", ".info"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(
				cache, "cache", "download",
				"example.com", "external", "@v", version+tc.ext,
			)

			original, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}

			// This is an isolated test cache, never the user's cache.
			if err := os.Chmod(source, 0600); err != nil {
				t.Fatal(err)
			}

			restore := func() error {
				if err := os.WriteFile(source, original, 0600); err != nil {
					return err
				}
				return os.Chmod(source, info.Mode().Perm())
			}

			// Restore even if a subsequent assertion fails.
			t.Cleanup(func() {
				if err := restore(); err != nil {
					t.Errorf("restore test cache artifact: %v", err)
				}
			})

			if err := os.WriteFile(
				source, []byte("invalid artifact content"), 0600,
			); err != nil {
				t.Fatal(err)
			}

			// Exercise the actual production capture while the
			// isolated cache contains a corrupted dependency.
			productionCalls := 0
			productionSpy := func(
				argv []string,
				executable string,
				dir string,
				env []string,
				stdout, stderr io.Writer,
			) (execution.Result, error) {
				productionCalls++
				return execution.Result{}, nil
			}

			productionOutput := filepath.Join(
				parent, "corrupted-"+strings.TrimPrefix(tc.ext, ".")+".rew",
			)

			code, diagnostic := captureV4StderrForTest(t, func() int {
				return captureCommandV4WithExecutor(
					[]string{"go", "build", "./..."},
					app,
					productionOutput,
					productionSpy,
				)
			})

			expectedStage := "discover offline module graph"
			if tc.ext == ".zip" {
				expectedStage = "capture external Go artifacts"
			}
			if !strings.Contains(diagnostic, expectedStage) {
				t.Fatalf(
					"corrupted %s: expected stage %q, got: %s",
					tc.ext, expectedStage, diagnostic,
				)
			}

			if code != 125 {
				t.Fatalf("corrupted %s: capture exit=%d", tc.ext, code)
			}
			if productionCalls != 0 {
				t.Fatalf(
					"corrupted %s: executor called %d times",
					tc.ext, productionCalls,
				)
			}
			if _, err := os.Lstat(productionOutput); !os.IsNotExist(err) {
				t.Fatalf(
					"corrupted %s: unexpected capsule: %v",
					tc.ext, err,
				)
			}

			badStore, err := capsule.NewArtifactStoreV3(
				capsule.MaxGoModuleTotalSizeV3,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer badStore.Close()

			_, err = capsule.ReadGoModuleArtifactSetToStoreV3(
				identities, sumData, cache, badStore,
			)
			if err == nil {
				t.Fatalf("corrupted %s artifact accepted", tc.ext)
			}
			t.Logf("correctly rejected: %v", err)
		})
	}

	t.Run("production rejects invalid pinned go.sum", func(t *testing.T) {
		sumPath := filepath.Join(app, "go.sum")

		original, err := os.ReadFile(sumPath)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(sumPath)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if err := os.WriteFile(sumPath, original, info.Mode().Perm()); err != nil {
				t.Errorf("restore pinned go.sum: %v", err)
			}
		})

		if err := os.WriteFile(
			sumPath, []byte("invalid go.sum\\n"), info.Mode().Perm(),
		); err != nil {
			t.Fatal(err)
		}

		calls := 0
		spy := func(
			argv []string,
			executable string,
			dir string,
			env []string,
			stdout, stderr io.Writer,
		) (execution.Result, error) {
			calls++
			return execution.Result{}, nil
		}

		badOutput := filepath.Join(parent, "invalid-sum.rew")
		code, diagnostic := captureV4StderrForTest(t, func() int {
			return captureCommandV4WithExecutor(
				[]string{"go", "build", "./..."},
				app,
				badOutput,
				spy,
			)
		})

		if !strings.Contains(
			diagnostic, "discover offline module graph",
		) {
			t.Fatalf(
				"invalid pinned go.sum: unexpected rejection stage: %s",
				diagnostic,
			)
		}

		if code != 125 {
			t.Fatalf("invalid go.sum: capture exit=%d", code)
		}
		if calls != 0 {
			t.Fatalf("invalid go.sum: executor called %d times", calls)
		}
		if _, err := os.Lstat(badOutput); !os.IsNotExist(err) {
			t.Fatalf("invalid go.sum: unexpected capsule: %v", err)
		}
	})

	t.Run("invalid go.sum", func(t *testing.T) {
		badStore, err := capsule.NewArtifactStoreV3(
			capsule.MaxGoModuleTotalSizeV3,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer badStore.Close()

		_, err = capsule.ReadGoModuleArtifactSetToStoreV3(
			identities,
			[]byte("invalid go.sum\\n"),
			cache,
			badStore,
		)
		if err == nil {
			t.Fatal("invalid go.sum accepted")
		}
		t.Logf("correctly rejected: %v", err)
	})
}

// captureV4StderrForTest records the production diagnostic.
// Call only from non-parallel tests because os.Stderr is process-global.
func captureV4StderrForTest(
	t *testing.T,
	run func() int,
) (code int, diagnostic string) {
	t.Helper()

	output, err := os.CreateTemp(t.TempDir(), "capture-stderr-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()

	original := os.Stderr
	os.Stderr = output
	defer func() {
		os.Stderr = original
	}()

	code = run()

	if err := output.Sync(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}

func TestCaptureV4MixedFailedBuildEndToEnd(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	lib := filepath.Join(parent, "lib")
	proxy := filepath.Join(parent, "proxy")
	cache := filepath.Join(parent, "cache")
	output := filepath.Join(parent, "failure.rew")

	t.Cleanup(func() {
		// Go's extracted module cache is intentionally read-only.
		// Restore directory and file write permissions before
		// testing.TempDir performs its recursive cleanup.
		err := filepath.Walk(cache, func(
			name string,
			info os.FileInfo,
			walkErr error,
		) error {
			if os.IsNotExist(walkErr) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if info.Mode().IsRegular() || info.IsDir() {
				return os.Chmod(name, info.Mode().Perm()|0700)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Errorf("restore module cache permissions: %v", err)
		}
	})

	write := func(name string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(app, "go.mod"), []byte(`module example.com/app

go 1.25

require (
	example.com/local-lib v0.0.0
	example.com/external v1.0.0
)

replace example.com/local-lib => ../lib
`))

	write(filepath.Join(app, "main.go"), []byte(`package main

import (
	_ "example.com/local-lib"
	_ "example.com/external"
)

func main() {}
`))

	write(filepath.Join(lib, "go.mod"), []byte(
		"module example.com/local-lib\n\ngo 1.25\n",
	))
	write(filepath.Join(lib, "lib.go"), []byte(
		"package lib\n\nfunc Value() {}\n",
	))

	const externalModule = "example.com/external"
	const version = "v1.0.0"

	externalMod := []byte(
		"module example.com/external\n\ngo 1.25\n",
	)

	proxyVersionDir := filepath.Join(
		proxy, "example.com", "external", "@v",
	)

	write(
		filepath.Join(proxyVersionDir, version+".mod"),
		externalMod,
	)
	write(
		filepath.Join(proxyVersionDir, version+".info"),
		[]byte(`{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`),
	)
	write(
		filepath.Join(proxyVersionDir, "list"),
		[]byte(version+"\n"),
	)

	var zipData bytes.Buffer
	zipWriter := zip.NewWriter(&zipData)

	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"go.mod", externalMod},
		{"external.go", []byte("package external\n\nfunc Value() {}\n")},
	} {
		name := externalModule + "@" + version + "/" + entry.name
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	write(
		filepath.Join(proxyVersionDir, version+".zip"),
		zipData.Bytes(),
	)

	goExecutable, err := capsule.ResolveGoExecutable()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOMODCACHE", cache)
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy))

	download := exec.Command(
		goExecutable, "mod", "download", externalModule+"@"+version,
	)
	download.Dir = app

	if data, err := download.CombinedOutput(); err != nil {
		t.Fatalf("populate cache from local proxy: %v\n%s", err, data)
	}

	t.Setenv("GOPROXY", "off")

	selected, err := capsule.ListGoModuleGraphV4WithExecutable(
		app, cache, goExecutable,
	)
	if err != nil {
		t.Fatalf("offline mixed graph discovery: %v", err)
	}

	localIndex, externalIndex, err :=
		classifyCaptureModuleGraphV4(selected, "../lib")
	if err != nil {
		t.Fatalf("valid mixed graph rejected: %v", err)
	}

	if localIndex < 0 || externalIndex < 0 ||
		localIndex == externalIndex {
		t.Fatalf(
			"expected local and external modules, got %d/%d",
			localIndex, externalIndex,
		)
	}

	if selected[localIndex].Path != "example.com/local-lib" ||
		selected[externalIndex].Path != externalModule ||
		selected[externalIndex].Version != version {
		t.Fatalf("unexpected mixed module identities: %+v", selected)
	}

	// Both dependencies are imported by app/main.go.
	// Introduce a real compiler failure in the local replacement.
	write(filepath.Join(lib, "lib.go"), []byte(
		"package lib\n\nfunc Broken( {\n",
	))

	code := captureCommandV4(
		[]string{"go", "build", "./..."},
		app,
		output,
	)

	if code != 1 {
		t.Fatalf("mixed capture exit=%d, want 1", code)
	}

	verified, err := capsule.ReadVerifiedToStoreV4(output)
	if err != nil {
		t.Fatalf("verify real mixed capsule: %v", err)
	}
	defer func() {
		if err := verified.Close(); err != nil {
			t.Errorf("close verified mixed capsule: %v", err)
		}
	}()

	m := verified.Manifest

	if m.SchemaVersion != capsule.SchemaVersionV4 {
		t.Fatalf("schema=%d, want v4", m.SchemaVersion)
	}

	if m.Execution.ExitCode != 1 ||
		!strings.Contains(m.Execution.Stderr, "lib.go") ||
		!strings.Contains(m.Execution.Stderr, "syntax error") ||
		strings.Contains(m.Execution.Stderr, "U+005C") {
		t.Fatalf(
			"unexpected captured build failure: exit=%d stderr=%q",
			m.Execution.ExitCode, m.Execution.Stderr,
		)
	}

	if len(m.GoSelectedModules) != 2 {
		t.Fatalf(
			"selected module count=%d, want 2",
			len(m.GoSelectedModules),
		)
	}

	if len(m.GoModules) != 1 ||
		m.GoModules[0].Path != externalModule ||
		m.GoModules[0].Version != version ||
		len(m.GoModules[0].Artifacts) != 3 {
		t.Fatalf(
			"unexpected external module metadata: %+v",
			m.GoModules,
		)
	}

	if len(m.LocalModules) != 1 ||
		m.LocalModules[0].ModulePath != "example.com/local-lib" ||
		m.LocalModules[0].SnapshotPath != "lib" {
		t.Fatalf(
			"unexpected local module metadata: %+v",
			m.LocalModules,
		)
	}

	found := make(map[string]bool)
	for _, file := range verified.Files {
		found[file.Path] = true
	}

	for _, required := range []string{
		"app/go.mod",
		"app/go.sum",
		"app/main.go",
		"lib/go.mod",
		"lib/lib.go",
	} {
		if !found[required] {
			t.Fatalf("mixed capsule missing %q", required)
		}
	}

	t.Log("real offline mixed build failure captured and verified")

	if os.Getenv("REW_DOCKER_TEST") != "1" {
		t.Log("Docker replay not requested; set REW_DOCKER_TEST=1")
		return
	}

	if os.Geteuid() == 0 {
		t.Skip("mixed Docker replay refuses root execution")
	}

	t.Log("Running real Mixed V4 CLI replay through offline Docker")

	stdoutPath := filepath.Join(parent, "mixed-replay-stdout.txt")
	stdoutFile, err := os.OpenFile(
		stdoutPath,
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutFile.Close()

	originalStdout := os.Stdout

	replayCode := func() int {
		os.Stdout = stdoutFile
		defer func() {
			os.Stdout = originalStdout
		}()

		return runReplay([]string{
			"--v4",
			"--allow-command",
			"--memory", "512m",
			"--show-stderr",
			output,
		})
	}()

	if _, err := stdoutFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	stdoutBytes, err := io.ReadAll(stdoutFile)
	if err != nil {
		t.Fatal(err)
	}

	summary := string(stdoutBytes)
	t.Logf("Real mixed capture-to-replay result:\n%s", summary)

	if replayCode != 0 {
		t.Fatalf(
			"mixed CLI replay exit=%d, want 0:\n%s",
			replayCode, summary,
		)
	}

	requireSummaryFields := func(label string, expected []string) {
		t.Helper()
		for _, line := range strings.Split(summary, "\n") {
			if !strings.HasPrefix(line, label) {
				continue
			}

			got := strings.Fields(line)
			if len(got) != len(expected) {
				t.Fatalf(
					"unexpected replay summary field %q: %q",
					label, line,
				)
			}
			for i := range expected {
				if got[i] != expected[i] {
					t.Fatalf(
						"unexpected replay summary %q: %q",
						label, line,
					)
				}
			}
			return
		}
		t.Fatalf("missing replay summary field %q:\n%s", label, summary)
	}

	requireSummaryFields(
		"Original exit code:",
		[]string{"Original", "exit", "code:", "1"},
	)
	requireSummaryFields(
		"Replay exit code:",
		[]string{"Replay", "exit", "code:", "1"},
	)
	requireSummaryFields(
		"Outcome:",
		[]string{"Outcome:", "reproduced"},
	)

	for _, required := range []string{
		"=== Original stderr ===",
		"=== Replay stderr ===",
		"../lib/lib.go:3:14: syntax error:",
		"Replay memory:",
		"512 MiB",
	} {
		if !strings.Contains(summary, required) {
			t.Fatalf(
				"mixed replay missing %q:\n%s",
				required, summary,
			)
		}
	}

	if strings.Count(summary, "syntax error:") < 2 {
		t.Fatalf(
			"original and replayed errors not both visible:\n%s",
			summary,
		)
	}

	t.Log("REAL MIXED CAPTURE -> CLI -> DOCKER REPLAY: REPRODUCED")

}
