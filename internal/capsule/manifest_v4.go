package capsule

// SchemaVersionV4 identifies the proposed local-replacement capsule format.
// Schema v4 is experimental and is not yet supported by capture or replay.
const SchemaVersionV4 = 4

// ManifestV4 is a draft schema-v4 representation.
//
// The final archive encoding, metadata completeness rules, and verification
// contract remain under development. This type must not be used to publish
// capsules until the complete v4 pipeline is implemented.
type ManifestV4 struct {
	SchemaVersion     int                  `json:"schema_version"`
	Execution         Execution            `json:"execution"`
	Platform          Platform             `json:"platform"`
	Workspace         Workspace            `json:"workspace"`
	Runtime           *Runtime             `json:"runtime"`
	GoBuildEnv        *GoBuildEnvironment  `json:"go_build_env"`
	GoModules         []GoModuleV3         `json:"go_modules,omitempty"`
	GoGraphInfoFiles  []GoGraphInfoFileV3  `json:"go_graph_info_files,omitempty"`
	GoGraphModFiles   []GoGraphModFileV3   `json:"go_graph_mod_files,omitempty"`
	GoSelectedModules []GoModuleIdentityV4 `json:"go_selected_modules"`
	LocalModules      []LocalModuleV4      `json:"local_modules"`
}

// GoModuleIdentityV4 identifies an original selected Go module
// and its optional effective local directory replacement.
type GoModuleIdentityV4 struct {
	Path    string                 `json:"path"`
	Version string                 `json:"version"`
	Replace *GoModuleReplacementV4 `json:"replace,omitempty"`
}

// GoModuleReplacementV4 records an effective local replacement.
// Path is the original relative path from the main module's go.mod.
// SnapshotPath locates its verified source tree inside the snapshot.
type GoModuleReplacementV4 struct {
	Path         string `json:"path"`
	SnapshotPath string `json:"snapshot_path"`
}

// LocalModuleV4 describes a captured local module source tree.
// Paths in Files are relative to this module's snapshot directory.
type LocalModuleV4 struct {
	ModulePath   string         `json:"module_path"`
	SnapshotPath string         `json:"snapshot_path"`
	Files        []FileMetadata `json:"files"`
}
