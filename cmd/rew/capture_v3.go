package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rewtools/rew/internal/capsule"
	"github.com/rewtools/rew/internal/execution"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

func captureCommandV3(
	argv []string,
	workspaceRoot string,
	includes []string,
	output string,
) int {
	return captureCommandV3WithEmbed(
		argv, workspaceRoot, includes, output, false,
	)
}

func captureCommandV3WithDiscovery(
	argv []string,
	workspaceRoot string,
	includes []string,
	output string,
	includeGoEmbed bool,
	includeGoSources bool,
) int {
	return captureCommandV3WithOptions(
		argv, workspaceRoot, includes, output,
		includeGoEmbed, includeGoSources,
	)
}

func captureCommandV3WithEmbed(
	argv []string,
	workspaceRoot string,
	includes []string,
	output string,
	includeGoEmbed bool,
) int {
	return captureCommandV3WithOptions(
		argv, workspaceRoot, includes, output,
		includeGoEmbed, false,
	)
}

func captureCommandV3WithOptions(
	argv []string,
	workspaceRoot string,
	includes []string,
	output string,
	includeGoEmbed bool,
	includeGoSources bool,
) int {
	if len(argv) == 0 || (!includeGoSources && len(includes) == 0) {
		fmt.Fprintln(os.Stderr,
			"rew: v3 requires a command and --include files")
		return 2
	}

	if err := checkSensitiveEnvironmentV3(os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "rew: v3 capture preflight: %v\n", err)
		return 125
	}

	if err := checkSensitiveWorkspacePathsV3(includes); err != nil {
		fmt.Fprintf(os.Stderr, "rew: v3 capture preflight: %v\n", err)
		return 125
	}

	goExecutable, err := capsule.ResolveGoExecutable()
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: resolve Go executable: %v\n", err)
		return 125
	}

	if err := capsule.ValidateGoWorkspaceV3WithExecutable(
		workspaceRoot, goExecutable,
	); err != nil {
		fmt.Fprintf(os.Stderr, "rew: Go workspace: %v\n", err)
		return 125
	}

	detectedRuntime := capsule.DetectRuntimeWithExecutable(
		argv, workspaceRoot, goExecutable,
	)

	if detectedRuntime == nil ||
		detectedRuntime.Kind != "go" ||
		!capsule.IsSupportedGoVersion(detectedRuntime.Version) {
		fmt.Fprintln(os.Stderr,
			"rew: v3 requires the supported Go runtime")
		return 125
	}

	goBuildEnv, err := capsule.InspectGoBuildEnvironmentWithExecutable(
		workspaceRoot, goExecutable,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: inspect Go environment: %v\n", err)
		return 125
	}

	if err := goBuildEnv.ValidatePortableGoBuild(); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: unsupported portable Go environment: %v\n", err)
		return 125
	}

	if err := checkCaptureOutputAvailableV3(output); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: capture output: %v\n", err)
		return 125
	}

	if includeGoSources {
		moduleCacheRoot, err := capsule.InspectGoModuleCacheWithExecutable(
			workspaceRoot, goExecutable,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rew: resolve Go module cache: %v\n", err)
			return 125
		}

		discovered, err := capsule.DiscoverGoSourceFilesV3WithExecutable(
			workspaceRoot, moduleCacheRoot, goExecutable, goBuildEnv,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rew: discover Go sources: %v\n", err)
			return 125
		}

		seen := make(map[string]struct{}, len(includes)+len(discovered)+2)
		merged := make([]string, 0, len(includes)+len(discovered)+2)

		add := func(name string) error {
			if _, exists := seen[name]; exists {
				return nil
			}
			if len(merged) >= capsule.MaxWorkspaceV3Files {
				return fmt.Errorf("schema-v3 workspace file limit exceeded (maximum %d files)", capsule.MaxWorkspaceV3Files)
			}
			seen[name] = struct{}{}
			merged = append(merged, name)
			return nil
		}

		for _, name := range includes {
			if _, exists := seen[name]; exists {
				fmt.Fprintf(os.Stderr, "rew: duplicate explicit include: %q\n", name)
				return 125
			}
			if err := add(name); err != nil {
				fmt.Fprintf(os.Stderr, "rew: %v\n", err)
				return 125
			}
		}

		for _, name := range discovered {
			if err := add(name); err != nil {
				fmt.Fprintf(os.Stderr, "rew: %v\n", err)
				return 125
			}
		}

		if err := add("go.mod"); err != nil {
			fmt.Fprintf(os.Stderr, "rew: %v\n", err)
			return 125
		}

		sumPath := filepath.Join(workspaceRoot, "go.sum")
		if info, err := os.Lstat(sumPath); err == nil {
			if !info.Mode().IsRegular() {
				fmt.Fprintln(os.Stderr, "rew: go.sum is not a regular file")
				return 125
			}
			if err := add("go.sum"); err != nil {
				fmt.Fprintf(os.Stderr, "rew: %v\n", err)
				return 125
			}
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "rew: inspect go.sum: %v\n", err)
			return 125
		}

		fmt.Fprintf(os.Stderr,
			"rew: Go source discovery selected %d source files\n",
			len(discovered),
		)
		fmt.Fprintln(os.Stderr,
			"rew: WARNING: selected source files will be stored in the capsule; review sensitive content before sharing")

		includes = merged
	}

	if includeGoEmbed {
		moduleCacheRoot, err := capsule.InspectGoModuleCacheWithExecutable(
			workspaceRoot, goExecutable,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rew: resolve Go module cache: %v\n", err)
			return 125
		}

		discovered, err := capsule.DiscoverGoEmbedFilesV3WithExecutable(
			workspaceRoot, moduleCacheRoot, goExecutable, goBuildEnv,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"rew: discover Go embedded files: %v\n", err)
			return 125
		}

		seen := make(map[string]struct{}, len(includes)+len(discovered))
		merged := make([]string, 0, len(includes)+len(discovered))

		for _, name := range includes {
			if _, exists := seen[name]; exists {
				fmt.Fprintf(os.Stderr,
					"rew: duplicate explicit include: %q\n", name)
				return 125
			}
			seen[name] = struct{}{}
			merged = append(merged, name)
		}

		added := 0
		for _, name := range discovered {
			if _, exists := seen[name]; exists {
				continue
			}
			if len(merged) >= capsule.MaxWorkspaceV3Files {
				fmt.Fprintf(os.Stderr,
					"rew: schema-v3 workspace file limit exceeded after Go embed discovery (maximum %d files)\n", capsule.MaxWorkspaceV3Files)
				return 125
			}
			seen[name] = struct{}{}
			merged = append(merged, name)
			added++
		}

		includes = merged
		fmt.Fprintf(os.Stderr,
			"rew: Go embed discovery selected %d additional files\n",
			added)
	}

	// Schema v3 counts the virtual empty go.sum as a workspace
	// file. Reserve its slot before executing the captured command.
	// A real go.sum must be included explicitly.
	if len(includes) >= capsule.MaxWorkspaceV3Files {
		hasIncludedGoSum := false
		for _, name := range includes {
			if name == "go.sum" {
				hasIncludedGoSum = true
				break
			}
		}
		if !hasIncludedGoSum {
			fmt.Fprintf(os.Stderr,
				"rew: schema-v3 workspace file limit leaves no room for virtual go.sum (maximum %d files)\n", capsule.MaxWorkspaceV3Files)
			return 125
		}
	}

	if err := checkSensitiveWorkspacePathsV3(includes); err != nil {
		fmt.Fprintf(os.Stderr, "rew: v3 capture preflight: %v\n", err)
		return 125
	}

	beforeFiles, err := capsule.ReadWorkspaceFilesV3(
		workspaceRoot,
		includes,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: read workspace before execution: %v\n", err)
		return 125
	}

	buildEnv := capsule.LocalGoToolchainEnv()

	result, err := execution.RunInDirWithExecutableEnv(
		argv,
		goExecutable,
		workspaceRoot,
		buildEnv,
		os.Stdout,
		os.Stderr,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: execute command: %v\n", err)
		return 125
	}

	if result.ExitCode == 0 {
		return 0
	}

	if result.Stderr == "" || result.StderrTruncated {
		fmt.Fprintln(os.Stderr,
			"rew: v3 requires complete non-empty failure stderr")
		return 125
	}

	if err := verifyCapturedWorkspaceV3(
		workspaceRoot, includes, beforeFiles,
	); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: workspace changed after execution: %v\n", err)
		return 125
	}

	files := beforeFiles

	var goMod []byte
	var goSum []byte
	var hasGoMod bool
	var hasGoSum bool

	for _, file := range files {
		switch file.Path {
		case "go.mod":
			goMod = file.Data
			hasGoMod = true
		case "go.sum":
			goSum = file.Data
			hasGoSum = true
		}
	}

	if !hasGoMod {
		fmt.Fprintln(os.Stderr,
			"rew: v3 requires --include go.mod")
		return 125
	}

	if !hasGoSum {
		if _, err := os.Lstat(filepath.Join(workspaceRoot, "go.sum")); err == nil {
			fmt.Fprintln(os.Stderr,
				"rew: existing go.sum must be explicitly included")
			return 125
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr,
				"rew: inspect go.sum: %v\n", err)
			return 125
		}
	}
	moduleCacheRoot, err := capsule.InspectGoModuleCacheWithExecutable(
		workspaceRoot, goExecutable,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rew: resolve Go module cache: %v\n", err)
		return 125
	}

	selectedModules, err := capsule.ListGoModuleGraphV3WithExecutable(
		workspaceRoot,
		moduleCacheRoot,
		goExecutable,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: resolve offline Go module graph: %v\n", err)
		return 125
	}

	// A dependency-free module may legitimately have no go.sum.
	// Preserve the v3 archive contract by adding a virtual empty
	// go.sum only after resolving the complete offline module graph.
	if !hasGoSum {
		if len(selectedModules) != 0 {
			fmt.Fprintln(os.Stderr,
				"rew: go.sum is required for external Go modules")
			return 125
		}

		if _, err := os.Lstat(filepath.Join(workspaceRoot, "go.sum")); err == nil {
			fmt.Fprintln(os.Stderr,
				"rew: go.sum appeared during module graph resolution")
			return 125
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr,
				"rew: inspect go.sum: %v\n", err)
			return 125
		}

		files = append(files, capsule.WorkspaceFile{
			Path: "go.sum",
			Data: []byte{},
			Mode: 0644,
		})
	}

	// The graph must describe the same included go.mod/go.sum bytes
	// that will be embedded in the capsule.
	for _, original := range []struct {
		name string
		data []byte
	}{
		{"go.mod", goMod},
	} {
		current, err := os.ReadFile(
			filepath.Join(workspaceRoot, original.name),
		)
		if err != nil || !bytes.Equal(current, original.data) {
			fmt.Fprintf(os.Stderr,
				"rew: %s changed during module graph resolution\n",
				original.name,
			)
			return 125
		}
	}

	if hasGoSum {
		current, err := os.ReadFile(filepath.Join(workspaceRoot, "go.sum"))
		if err != nil || !bytes.Equal(current, goSum) {
			fmt.Fprintln(os.Stderr,
				"rew: go.sum changed during module graph resolution")
			return 125
		}
	}

	buildModules, err := capsule.ListGoBuildModulesV3WithExecutable(
		workspaceRoot, moduleCacheRoot, selectedModules, goExecutable,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: discover build dependencies: %v\n", err)
		return 125
	}

	store, err := capsule.NewArtifactStoreV3(
		capsule.MaxGoModuleTotalSizeV3 +
			capsule.MaxGoGraphModTotalSizeV3 +
			capsule.MaxGoGraphInfoTotalSizeV3,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: create artifact store: %v\n", err)
		return 125
	}
	defer store.Close()

	goModules, err :=
		capsule.ReadGoModuleArtifactSetToStoreV3(
			buildModules,
			goSum,
			moduleCacheRoot,
			store,
		)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: capture Go module artifacts: %v\n", err)
		return 125
	}

	parsedGoMod, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil || parsedGoMod.Module == nil {
		fmt.Fprintf(os.Stderr,
			"rew: invalid main go.mod: %v\n", err)
		return 125
	}

	mainModulePath := parsedGoMod.Module.Mod.Path
	if err := module.CheckPath(mainModulePath); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: invalid main module path: %v\n", err)
		return 125
	}

	graphIdentities, err := capsule.ListGoGraphModRequirementsV3WithExecutable(
		workspaceRoot,
		moduleCacheRoot,
		mainModulePath,
		selectedModules,
		goExecutable,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: discover Go graph metadata: %v\n", err)
		return 125
	}

	graphModIdentities, err := capsule.SelectVerifiedGoGraphModsV3(
		selectedModules, goModules, graphIdentities, goSum,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: select graph metadata: %v\n", err)
		return 125
	}

	graphFiles, graphArtifacts, err :=
		capsule.ReadGoGraphModFilesForCaptureV3(
			graphModIdentities, goSum, moduleCacheRoot,
		)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: capture Go graph metadata: %v\n", err)
		return 125
	}

	infoFiles, infoArtifacts, err :=
		capsule.ReadGoGraphInfoFilesForCaptureV3(
			selectedModules, goModules, moduleCacheRoot,
		)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: capture graph info: %v\n", err)
		return 125
	}

	// Graph artifacts are appended to the same disk-backed store
	// after both metadata categories have been collected.

	selectedManifest := capsule.Manifest{
		GoModules:         goModules,
		GoSelectedModules: selectedModules,
		GoGraphInfoFiles:  infoFiles,
	}
	if err := capsule.ValidateGoSelectedModuleConsistencyV3(
		goMod, selectedManifest,
	); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: inconsistent Go module graph: %v\n", err)
		return 125
	}

	if err := verifyCapturedWorkspaceV3(
		workspaceRoot, includes, beforeFiles,
	); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: workspace changed before packaging: %v\n", err)
		return 125
	}

	manifest := capsule.Manifest{
		SchemaVersion:     capsule.SchemaVersionV3,
		Runtime:           detectedRuntime,
		GoBuildEnv:        &goBuildEnv,
		GoModules:         goModules,
		GoSelectedModules: selectedModules,
		GoGraphModFiles:   graphFiles,
		GoGraphInfoFiles:  infoFiles,
		Execution: capsule.Execution{
			Argv:            result.Argv,
			WorkingDir:      ".",
			ExitCode:        result.ExitCode,
			Stdout:          result.Stdout,
			Stderr:          result.Stderr,
			StdoutTruncated: result.StdoutTruncated,
			StderrTruncated: result.StderrTruncated,
		},
		Platform: capsule.Platform{
			OS:           runtime.GOOS,
			Architecture: runtime.GOARCH,
		},
	}

	if err := capsule.AddGoGraphArtifactsToStoreV3(
		manifest,
		goSum,
		graphArtifacts,
		infoArtifacts,
		store,
	); err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: store Go graph artifacts: %v\n", err)
		return 125
	}

	f, err := os.OpenFile(
		output,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"rew: create capsule: %v\n", err)
		return 125
	}

	writeErr := capsule.WriteWorkspaceFromStoreV3(
		f,
		manifest,
		files,
		store,
	)
	closeErr := f.Close()

	if writeErr != nil || closeErr != nil {
		removeErr := os.Remove(output)
		fmt.Fprintf(os.Stderr,
			"rew: write v3 capsule: write=%v close=%v remove=%v\n",
			writeErr,
			closeErr,
			removeErr,
		)
		return 125
	}

	fmt.Fprintf(os.Stderr,
		"rew: capsule v3 saved to %s\n", output)

	return result.ExitCode
}
