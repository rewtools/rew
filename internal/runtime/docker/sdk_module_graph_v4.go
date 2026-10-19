package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/mod/module"
)

type dockerGoModuleReplacementV4 struct {
	Path      string          `json:"Path"`
	Version   string          `json:"Version"`
	Dir       string          `json:"Dir"`
	GoMod     string          `json:"GoMod"`
	GoVersion string          `json:"GoVersion"`
	Error     json.RawMessage `json:"Error"`
}

type dockerListedGoModuleV4 struct {
	Path      string          `json:"Path"`
	Version   string          `json:"Version"`
	Main      bool            `json:"Main"`
	Replace   json.RawMessage `json:"Replace"`
	Error     json.RawMessage `json:"Error"`
	Dir       string          `json:"Dir"`
	GoMod     string          `json:"GoMod"`
	GoVersion string          `json:"GoVersion"`
}

// verifyDockerGoModuleGraphV4 checks the resolved Go graph for
// the schema-v4 single-local-replacement profile.
//
// The expected main module, local module and directory must come
// from an independently verified V4 capsule, not untrusted CLI input.
func verifyDockerGoModuleGraphV4(
	data []byte,
	mainModule string,
	localModule string,
	localDirectory string,
) error {
	if len(data) == 0 || len(data) > 1<<20 {
		return fmt.Errorf("docker SDK: invalid v4 module graph size")
	}

	if err := module.CheckPath(mainModule); err != nil {
		return fmt.Errorf("docker SDK: invalid v4 main module: %w", err)
	}
	if err := module.CheckPath(localModule); err != nil {
		return fmt.Errorf("docker SDK: invalid v4 local module: %w", err)
	}
	if mainModule == localModule {
		return fmt.Errorf("docker SDK: identical main and local modules")
	}
	if localDirectory != "/workspace/lib" {
		return fmt.Errorf("docker SDK: unsupported v4 local directory")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	seenMain := false
	seenLocal := false
	count := 0

	for {
		var item dockerListedGoModuleV4

		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf(
				"docker SDK: decode v4 module graph: %w",
				err,
			)
		}

		count++
		if count > 2 {
			return fmt.Errorf(
				"docker SDK: unexpected additional v4 module",
			)
		}

		if len(item.Error) != 0 &&
			string(item.Error) != "null" {
			return fmt.Errorf("docker SDK: v4 module graph error")
		}

		if item.Main {
			if seenMain ||
				item.Path != mainModule ||
				item.Version != "" ||
				item.Dir != "/workspace/app" ||
				item.GoMod != "/workspace/app/go.mod" ||
				(len(item.Replace) != 0 &&
					string(item.Replace) != "null") {
				return fmt.Errorf(
					"docker SDK: invalid v4 main module",
				)
			}
			seenMain = true
			continue
		}

		if seenLocal ||
			item.Path != localModule ||
			item.Version != "v0.0.0" ||
			item.Dir != localDirectory ||
			item.GoMod != localDirectory+"/go.mod" {
			return fmt.Errorf(
				"docker SDK: unexpected v4 selected module %q@%q",
				item.Path, item.Version,
			)
		}

		if len(item.Replace) == 0 ||
			string(item.Replace) == "null" {
			return fmt.Errorf(
				"docker SDK: missing v4 local replacement",
			)
		}

		var replacement dockerGoModuleReplacementV4
		replacementDecoder := json.NewDecoder(
			bytes.NewReader(item.Replace),
		)
		replacementDecoder.DisallowUnknownFields()

		if err := replacementDecoder.Decode(
			&replacement,
		); err != nil {
			return fmt.Errorf(
				"docker SDK: decode v4 replacement: %w",
				err,
			)
		}

		var trailing any
		if err := replacementDecoder.Decode(&trailing); err != io.EOF {
			return fmt.Errorf(
				"docker SDK: trailing v4 replacement data",
			)
		}

		if (len(replacement.Error) != 0 &&
			string(replacement.Error) != "null") ||
			replacement.Path != "../lib" ||
			replacement.Version != "" ||
			replacement.Dir != localDirectory ||
			replacement.GoMod != localDirectory+"/go.mod" {
			return fmt.Errorf(
				"docker SDK: invalid v4 local replacement",
			)
		}

		seenLocal = true
	}

	if !seenMain || !seenLocal || count != 2 {
		return fmt.Errorf(
			"docker SDK: incomplete v4 local module graph",
		)
	}

	return nil
}
