package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/execution"
	"golang.org/x/mod/modfile"
)

type captureV4Executor func(
	argv []string,
	executable string,
	dir string,
	env []string,
	stdout, stderr io.Writer,
) (execution.Result, error)

func captureCommandV4(
	argv []string,
	workspaceRoot string,
	output string,
) int {
	return captureCommandV4WithExecutor(
		argv,
		workspaceRoot,
		output,
		execution.RunInDirWithExecutableEnv,
	)
}

func captureCommandV4WithExecutor(
	argv []string,
	workspaceRoot string,
	output string,
	execute captureV4Executor,
) int {
	fail := func(format string, args ...any) int {
		fmt.Fprintf(os.Stderr, "rew: v4 capture: "+format+"\n", args...)
		return 125
	}

	if len(argv) != 3 ||
		argv[0] != "go" ||
		argv[1] != "build" ||
		argv[2] != "./..." {
		return fail("V4 MVP supports exactly: go build ./...")
	}

	if err := checkSensitiveEnvironmentV3(os.Environ()); err != nil {
		return fail("environment preflight: %v", err)
	}

	if err := checkCaptureOutputAvailableV3(output); err != nil {
		return fail("output preflight: %v", err)
	}

	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fail("absolute workspace path: %v", err)
	}

	goExecutable, err := capsule.ResolveGoExecutable()
	if err != nil {
		return fail("resolve Go executable: %v", err)
	}

	if err := capsule.ValidateGoWorkspaceV3WithExecutable(
		root, goExecutable,
	); err != nil {
		return fail("Go workspace preflight: %v", err)
	}

	runtimeSpec := capsule.DetectRuntimeWithExecutable(
		argv, root, goExecutable,
	)
	if runtimeSpec == nil ||
		runtimeSpec.Kind != "go" ||
		!capsule.IsSupportedGoVersion(runtimeSpec.Version) {
		return fail("unsupported Go runtime")
	}

	environment, err :=
		capsule.InspectGoBuildEnvironmentWithExecutable(
			root, goExecutable,
		)
	if err != nil {
		return fail("inspect Go environment: %v", err)
	}

	if environment.GOFLAGS != "" {
		return fail("GOFLAGS must be empty")
	}

	target := capsule.GoBuildEnvironment{
		GOOS:       "linux",
		GOARCH:     "amd64",
		CGOEnabled: "0",
	}
	if err := target.ValidatePortableGoBuild(); err != nil {
		return fail("portable target: %v", err)
	}

	moduleFile, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fail("read main go.mod: %v", err)
	}

	parsed, err := modfile.Parse("go.mod", moduleFile, nil)
	if err != nil || parsed.Module == nil {
		return fail("invalid main go.mod: %v", err)
	}

	if len(parsed.Replace) != 1 ||
		parsed.Replace[0].Old.Version != "" ||
		parsed.Replace[0].New.Version != "" {
		return fail("exactly one unversioned local replace is required")
	}

	replacePath := parsed.Replace[0].New.Path

	cacheRoot, err := capsule.InspectGoModuleCacheWithExecutable(
		root, goExecutable,
	)
	if err != nil {
		return fail("inspect Go module cache: %v", err)
	}

	// Keep both physical module roots pinned through execution
	// and packaging. This is not an atomic filesystem snapshot.
	session, err := capsule.OpenPinnedLocalModuleSessionV4(
		root, replacePath,
	)
	if err != nil {
		return fail("open pinned module roots: %v", err)
	}
	defer session.Close()

	selected, err := capsule.ListGoModuleGraphV4WithExecutable(
		root, cacheRoot, goExecutable,
	)
	if err != nil {
		return fail("discover offline module graph: %v", err)
	}

	localIndex, externalIndex, err := classifyCaptureModuleGraphV4(
		selected, replacePath,
	)
	if err != nil {
		return fail("unsupported module graph: %v", err)
	}

	selected[localIndex].Replace.SnapshotPath = "lib"

	mainPaths, localPaths, err :=
		capsule.DiscoverLocalGoSourcesV4WithExecutable(
			root,
			replacePath,
			cacheRoot,
			goExecutable,
			target,
		)
	if err != nil {
		return fail("discover local Go sources: %v", err)
	}

	if externalIndex != -1 {
		// The mixed writer requires the actual pinned go.sum bytes.
		// Do not synthesize a virtual or empty go.sum.
		for _, name := range mainPaths {
			if name == "go.sum" {
				return fail("duplicate mixed v4 go.sum discovery")
			}
		}
		mainPaths = append(mainPaths, "go.sum")
		sort.Strings(mainPaths)
	}

	if err := checkSensitiveWorkspacePathsV3(mainPaths); err != nil {
		return fail("main module sensitive path: %v", err)
	}
	if err := checkSensitiveWorkspacePathsV3(localPaths); err != nil {
		return fail("local module sensitive path: %v", err)
	}

	before, err := session.Read(mainPaths, localPaths)
	if err != nil {
		return fail("read pinned workspace: %v", err)
	}

	var pinnedMainGoMod []byte
	for _, file := range before {
		if file.Path == "app/go.mod" {
			pinnedMainGoMod = file.Data
			break
		}
	}

	if !bytes.Equal(pinnedMainGoMod, moduleFile) {
		return fail("main go.mod changed during discovery")
	}

	if err := capsule.ValidateGoModuleConsistencyV4(
		pinnedMainGoMod, selected,
	); err != nil {
		return fail("module graph consistency: %v", err)
	}

	// Keep verified artifacts available until capture finishes.
	// The mixed path remains fail-closed below.
	var mixedStore *capsule.ArtifactStoreV3
	var mixedGoModules []capsule.GoModuleV3

	if externalIndex != -1 {
		// Use only the bytes already read through the pinned root.
		// Never reopen go.sum by its mutable filesystem pathname.
		var pinnedGoSum []byte
		foundGoSum := false

		for _, file := range before {
			if file.Path != "app/go.sum" {
				continue
			}
			if foundGoSum {
				return fail("duplicate pinned mixed go.sum")
			}
			foundGoSum = true
			pinnedGoSum = file.Data
		}

		if !foundGoSum {
			return fail("missing pinned mixed go.sum")
		}

		externalModules, err := captureExternalModuleIdentitiesV4(
			selected, replacePath,
		)
		if err != nil {
			return fail("select external Go module: %v", err)
		}
		if len(externalModules) != 1 {
			return fail("expected exactly one external Go module")
		}

		mixedStore, err = capsule.NewArtifactStoreV3(
			capsule.MaxGoModuleTotalSizeV3,
		)
		if err != nil {
			return fail("create mixed artifact store: %v", err)
		}
		defer mixedStore.Close()

		mixedGoModules, err = capsule.ReadGoModuleArtifactSetToStoreV3(
			externalModules,
			pinnedGoSum,
			cacheRoot,
			mixedStore,
		)
		if err != nil {
			return fail("capture external Go artifacts: %v", err)
		}

		if err := capsule.VerifyMixedGoModuleArtifactsV4(
			selected,
			mixedGoModules,
			pinnedGoSum,
			mixedStore,
		); err != nil {
			return fail("verify mixed external Go artifacts: %v", err)
		}

	}

	fmt.Fprintln(
		os.Stderr,
		"rew: WARNING: v4 capsule embeds source files from both local modules; review sensitive content before sharing",
	)

	// Use the same explicit Linux build target that V4 replay uses.
	// Do not modify the parent process environment.
	env := make([]string, 0, len(os.Environ())+8)
	for _, entry := range capsule.LocalGoToolchainEnv() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GOOS", "GOARCH", "CGO_ENABLED",
			"GOFLAGS", "GOWORK", "GOPROXY", "GOSUMDB":
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
		"GOFLAGS=",
		"GOWORK=off",
		"GOPROXY=off",
		"GOSUMDB=off",
	)

	result, err := execute(
		argv, goExecutable, root, env,
		os.Stdout, os.Stderr,
	)
	if err != nil {
		return fail("execute Go build: %v", err)
	}

	if result.ExitCode == 0 {
		return 0
	}

	if result.Stderr == "" || result.StderrTruncated {
		return fail("complete non-empty failure stderr is required")
	}

	if err := session.Verify(
		mainPaths, localPaths, before,
	); err != nil {
		return fail("workspace changed after execution: %v", err)
	}

	// Revalidate the selected graph after execution as well.
	afterGraph, err := capsule.ListGoModuleGraphV4WithExecutable(
		root, cacheRoot, goExecutable,
	)
	if err != nil {
		return fail("recheck offline module graph: %v", err)
	}

	if !captureV4GraphUnchanged(selected, afterGraph) {
		return fail("Go module graph changed during capture")
	}

	if err := session.Verify(
		mainPaths, localPaths, before,
	); err != nil {
		return fail("workspace changed before packaging: %v", err)
	}

	manifest := capsule.ManifestV4{
		SchemaVersion: capsule.SchemaVersionV4,
		Execution: capsule.Execution{
			Argv:            result.Argv,
			WorkingDir:      "app",
			ExitCode:        result.ExitCode,
			Stdout:          result.Stdout,
			Stderr:          result.Stderr,
			StdoutTruncated: result.StdoutTruncated,
			StderrTruncated: result.StderrTruncated,
		},
		Platform: capsule.Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
		Runtime:           runtimeSpec,
		GoBuildEnv:        &target,
		GoSelectedModules: selected,
		GoModules:         mixedGoModules,
		LocalModules: []capsule.LocalModuleV4{
			{
				ModulePath:   selected[localIndex].Path,
				SnapshotPath: "lib",
			},
		},
	}

	// Stream privately and publish only after a successful write,
	// sync, and close. Existing output must never be replaced.
	err = publishCaptureV4(output, func(w io.Writer) error {
		if externalIndex == -1 {
			return capsule.WriteWorkspaceV4(w, manifest, before)
		}

		if mixedStore == nil || len(mixedGoModules) != 1 {
			return fmt.Errorf(
				"missing verified mixed Go module artifacts",
			)
		}

		return capsule.WriteWorkspaceFromStoreV4(
			w, manifest, before, mixedStore,
		)
	})
	if err != nil {
		return fail("write capsule: %v", err)
	}

	fmt.Fprintf(
		os.Stderr,
		"rew: capsule v4 saved to %s\n", output,
	)

	return result.ExitCode
}
