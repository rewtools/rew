package capsule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/mod/module"
)

// ParseGoModuleGraphV4 parses a bounded stream of JSON objects from
// `go list -m -json all`.
//
// It returns selected external module identities and preserves
// effective local replacement paths. Snapshot paths are intentionally
// left unset until the capture layout has been validated.
//
// This parser does not verify filesystem contents or Go module
// replacement relationships against go.mod.
func ParseGoModuleGraphV4(data []byte) ([]GoModuleIdentityV4, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, fmt.Errorf("capsule: invalid v4 Go module graph size")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	modules := make([]GoModuleIdentityV4, 0)
	seen := make(map[string]bool)
	mainCount := 0
	replacementCount := 0

	for {
		var item goListModuleV3

		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("capsule: decode v4 Go module graph: %w", err)
		}

		if len(item.Error) != 0 &&
			string(item.Error) != "null" {
			return nil, fmt.Errorf("capsule: v4 Go module graph contains an error")
		}

		if item.Main {
			mainCount++

			if item.Path == "" ||
				item.Version != "" ||
				len(item.Replace) != 0 &&
					string(item.Replace) != "null" {
				return nil, fmt.Errorf("capsule: invalid v4 main module")
			}

			continue
		}

		if err := module.Check(item.Path, item.Version); err != nil {
			return nil, fmt.Errorf("capsule: invalid v4 module identity: %w", err)
		}

		if seen[item.Path] {
			return nil, fmt.Errorf("capsule: duplicate v4 module path: %s", item.Path)
		}
		seen[item.Path] = true

		selected := GoModuleIdentityV4{
			Path:    item.Path,
			Version: item.Version,
		}

		if len(item.Replace) != 0 &&
			string(item.Replace) != "null" {
			var replacement struct {
				Path      string `json:"Path"`
				Version   string `json:"Version"`
				Dir       string `json:"Dir"`
				GoMod     string `json:"GoMod"`
				GoVersion string `json:"GoVersion"`
			}

			replaceDecoder := json.NewDecoder(bytes.NewReader(item.Replace))
			replaceDecoder.DisallowUnknownFields()

			if err := replaceDecoder.Decode(&replacement); err != nil {
				return nil, fmt.Errorf(
					"capsule: invalid v4 replacement JSON: %w",
					err,
				)
			}

			var extra json.RawMessage
			if err := replaceDecoder.Decode(&extra); err != io.EOF {
				return nil, fmt.Errorf(
					"capsule: invalid trailing v4 replacement JSON",
				)
			}

			if replacement.Path == "" ||
				replacement.Version != "" ||
				strings.HasPrefix(replacement.Path, "/") ||
				strings.ContainsRune(replacement.Path, '\\') ||
				strings.ContainsRune(replacement.Path, 0) {
				return nil, fmt.Errorf(
					"capsule: unsupported v4 module replacement",
				)
			}

			// Reject unsupported path spellings before snapshot mapping.
			// Parent traversals are supported only as a leading prefix.
			components := strings.Split(replacement.Path, "/")
			for index, component := range components {
				switch component {
				case "":
					return nil, fmt.Errorf("capsule: empty v4 replacement path component")
				case ".":
					if index != 0 {
						return nil, fmt.Errorf("capsule: noncanonical v4 replacement path")
					}
				case "..":
					if index > 0 && components[index-1] != ".." &&
						components[index-1] != "." {
						return nil, fmt.Errorf("capsule: noncanonical v4 replacement path")
					}
				}
			}

			replacementCount++
			if replacementCount > 1 {
				return nil, fmt.Errorf(
					"capsule: v4 MVP supports one local replacement",
				)
			}

			selected.Replace = &GoModuleReplacementV4{
				Path: replacement.Path,
			}
		}

		modules = append(modules, selected)

		if len(modules) > MaxGoSelectedModulesV3 {
			return nil, fmt.Errorf("capsule: too many v4 selected modules")
		}
	}

	if mainCount != 1 {
		return nil, fmt.Errorf("capsule: expected exactly one v4 main module")
	}

	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Path < modules[j].Path
	})

	return modules, nil
}
