package capsule

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// DiscoverLocalGoSourcesV4WithExecutable discovers source and embedded
// files for both the main Go module and one sibling local module.
//
// The V3 discovery runners execute go list independently in each
// module, using their existing offline and bounded execution rules.
//
// The caller must separately validate the Go replacement graph.
// Discovery is not an atomic filesystem snapshot or sandbox.
func DiscoverLocalGoSourcesV4WithExecutable(
	mainRoot string,
	replacePath string,
	moduleCacheRoot string,
	goExecutable string,
	sourceTarget GoBuildEnvironment,
) (mainFiles []string, localFiles []string, err error) {
	if mainRoot == "" || replacePath == "" ||
		moduleCacheRoot == "" || !filepath.IsAbs(goExecutable) {
		return nil, nil, fmt.Errorf(
			"capsule: invalid v4 local source discovery input",
		)
	}

	session, err := OpenPinnedLocalModuleSessionV4(
		mainRoot, replacePath,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: open v4 discovery roots: %w", err,
		)
	}
	defer func() {
		err = errors.Join(err, session.Close())
		if err != nil {
			mainFiles = nil
			localFiles = nil
		}
	}()

	// Resolve the sibling path using the same parent as the main
	// module. The pinned session independently verifies root identity.
	physicalMain, err := filepath.EvalSymlinks(mainRoot)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: resolve main Go module: %w", err,
		)
	}

	physicalMain, err = filepath.Abs(physicalMain)
	if err != nil {
		return nil, nil, err
	}

	physicalLocal := filepath.Clean(
		filepath.Join(physicalMain, replacePath),
	)

	if err := session.VerifyRootIdentity(); err != nil {
		return nil, nil, err
	}

	// Read go.mod through the pinned root instead of reopening
	// the module by its mutable physical pathname.
	moduleFiles, err := session.Read(
		[]string{"go.mod"},
		[]string{"go.mod"},
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: read v4 module manifests: %w", err,
		)
	}

	var mainGoMod, localGoMod []byte
	for _, file := range moduleFiles {
		switch file.Path {
		case "app/go.mod":
			mainGoMod = file.Data
		case "lib/go.mod":
			localGoMod = file.Data
		}
	}
	if len(mainGoMod) == 0 || len(localGoMod) == 0 {
		return nil, nil, fmt.Errorf(
			"capsule: missing v4 module manifest",
		)
	}

	parsed, err := modfile.Parse("go.mod", mainGoMod, nil)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: parse main v4 go.mod: %w", err,
		)
	}

	if parsed.Module == nil ||
		len(parsed.Replace) != 1 ||
		parsed.Replace[0].Old.Version != "" ||
		parsed.Replace[0].New.Version != "" ||
		parsed.Replace[0].New.Path != replacePath {
		return nil, nil, fmt.Errorf(
			"capsule: v4 replacePath does not match main go.mod",
		)
	}

	replacedModulePath := parsed.Replace[0].Old.Path

	required := false
	for _, entry := range parsed.Require {
		if entry.Mod.Path == replacedModulePath {
			required = true
			break
		}
	}
	if !required {
		return nil, nil, fmt.Errorf(
			"capsule: replaced v4 module is not required",
		)
	}

	localParsed, err := modfile.Parse(
		"lib/go.mod", localGoMod, nil,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: parse local v4 go.mod: %w", err,
		)
	}

	if localParsed.Module == nil ||
		localParsed.Module.Mod.Path != replacedModulePath ||
		parsed.Module.Mod.Path == replacedModulePath {
		return nil, nil, fmt.Errorf(
			"capsule: local v4 module identity mismatch",
		)
	}

	// Validate the actual module selection used by the Go toolchain,
	// not merely the replacement declared in the two go.mod files.
	selected, err := ListGoModuleGraphV4WithExecutable(
		physicalMain,
		moduleCacheRoot,
		goExecutable,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: list v4 effective module graph: %w", err,
		)
	}

	if err := ValidateGoModuleConsistencyV4(mainGoMod, selected); err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: validate v4 effective module graph: %w", err,
		)
	}

	localMatches := 0
	for _, module := range selected {
		if module.Replace == nil {
			continue
		}
		if module.Path != replacedModulePath ||
			module.Replace.Path != replacePath {
			return nil, nil, fmt.Errorf(
				"capsule: unexpected v4 effective local replacement",
			)
		}
		localMatches++
	}
	if localMatches != 1 {
		return nil, nil, fmt.Errorf(
			"capsule: expected exactly one v4 effective replacement",
		)
	}

	if err := session.VerifyRootIdentity(); err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: v4 roots changed during graph discovery: %w", err,
		)
	}

	mainSources, err := DiscoverGoSourceFilesV3WithExecutable(
		physicalMain,
		moduleCacheRoot,
		goExecutable,
		sourceTarget,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: discover main Go sources: %w", err,
		)
	}

	localSources, err := DiscoverGoSourceFilesV3WithExecutable(
		physicalLocal,
		moduleCacheRoot,
		goExecutable,
		sourceTarget,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: discover local Go sources: %w", err,
		)
	}

	mainEmbeds, err := DiscoverGoEmbedFilesV3WithExecutable(
		physicalMain,
		moduleCacheRoot,
		goExecutable,
		sourceTarget,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: discover main Go embeds: %w", err,
		)
	}

	localEmbeds, err := DiscoverGoEmbedFilesV3WithExecutable(
		physicalLocal,
		moduleCacheRoot,
		goExecutable,
		sourceTarget,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: discover local Go embeds: %w", err,
		)
	}

	mainFiles, localFiles, err = MergeLocalGoSourcesV4(
		append(mainSources, mainEmbeds...),
		append(localSources, localEmbeds...),
	)
	if err != nil {
		return nil, nil, err
	}

	// Detect in-place or replacement changes to either go.mod
	// during the independently executed Go discovery commands.
	// This is a consistency check, not an atomic snapshot.
	if err := session.Verify(
		[]string{"go.mod"},
		[]string{"go.mod"},
		moduleFiles,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"capsule: v4 module manifests changed during discovery: %w",
			err,
		)
	}

	return mainFiles, localFiles, nil
}
