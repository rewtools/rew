package capsule

// ReadPinnedLocalModulesV4 captures explicitly selected source files
// using one short-lived pinned session.
//
// Capture workflows that need to verify the same physical directories
// later must hold their own PinnedLocalModuleSessionV4 open and call
// its Read and Verify methods.
func ReadPinnedLocalModulesV4(
	mainRoot string,
	replacePath string,
	mainPaths []string,
	localPaths []string,
) ([]WorkspaceFile, error) {
	if len(mainPaths) > MaxWorkspaceV3Files ||
		len(localPaths) > MaxWorkspaceV3Files-len(mainPaths) {
		return nil, errTooManyLocalModuleFilesV4()
	}

	session, err := OpenPinnedLocalModuleSessionV4(
		mainRoot, replacePath,
	)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	return session.Read(mainPaths, localPaths)
}
