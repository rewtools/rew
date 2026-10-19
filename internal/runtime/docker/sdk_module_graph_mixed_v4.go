package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/mod/module"
)

// verifyDockerGoModuleGraphMixedV4 validates the initial
// mixed-dependency V4 profile: one main module, one local
// replacement, and one exact external module.
//
// Expected identities must originate from verified capsule data.
// This validator does not replace the local-only V4 validator.
func verifyDockerGoModuleGraphMixedV4(
	data []byte,
	mainModule string,
	localModule string,
	localDirectory string,
	externalModule string,
	externalVersion string,
) error {
	if len(data) == 0 || len(data) > 1<<20 {
		return fmt.Errorf("docker SDK: invalid mixed v4 graph size")
	}

	if err := module.CheckPath(mainModule); err != nil {
		return fmt.Errorf("docker SDK: invalid main module: %w", err)
	}
	if err := module.Check(localModule, "v0.0.0"); err != nil {
		return fmt.Errorf("docker SDK: invalid local module: %w", err)
	}
	if err := module.Check(externalModule, externalVersion); err != nil {
		return fmt.Errorf("docker SDK: invalid external module: %w", err)
	}

	if mainModule == localModule ||
		mainModule == externalModule ||
		localModule == externalModule {
		return fmt.Errorf("docker SDK: duplicate mixed v4 identities")
	}
	if localDirectory != "/workspace/lib" {
		return fmt.Errorf("docker SDK: unsupported local directory")
	}

	if err := rejectDuplicateMixedV4JSONKeys(data); err != nil {
		return fmt.Errorf(
			"docker SDK: invalid mixed v4 JSON: %w", err,
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	seenMain := false
	seenLocal := false
	seenExternal := false
	count := 0

	for {
		var item dockerListedGoModuleV4

		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf(
				"docker SDK: decode mixed v4 graph: %w", err,
			)
		}

		count++
		if count > 3 {
			return fmt.Errorf(
				"docker SDK: unexpected additional mixed v4 module",
			)
		}

		if len(item.Error) != 0 &&
			string(item.Error) != "null" {
			return fmt.Errorf("docker SDK: mixed v4 graph error")
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
					"docker SDK: invalid mixed v4 main module",
				)
			}
			seenMain = true
			continue
		}

		switch item.Path {
		case localModule:
			if seenLocal ||
				item.Version != "v0.0.0" ||
				item.Dir != localDirectory ||
				item.GoMod != localDirectory+"/go.mod" {
				return fmt.Errorf(
					"docker SDK: invalid mixed v4 local module",
				)
			}

			if len(item.Replace) == 0 ||
				string(item.Replace) == "null" {
				return fmt.Errorf(
					"docker SDK: missing mixed v4 local replacement",
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
					"docker SDK: decode mixed v4 replacement: %w",
					err,
				)
			}

			var trailing any
			if err := replacementDecoder.Decode(
				&trailing,
			); err != io.EOF {
				return fmt.Errorf(
					"docker SDK: trailing mixed v4 replacement data",
				)
			}

			if (len(replacement.Error) != 0 &&
				string(replacement.Error) != "null") ||
				replacement.Path != "../lib" ||
				replacement.Version != "" ||
				replacement.Dir != localDirectory ||
				replacement.GoMod != localDirectory+"/go.mod" {
				return fmt.Errorf(
					"docker SDK: invalid mixed v4 local replacement",
				)
			}

			seenLocal = true

		case externalModule:
			escapedPath, err := module.EscapePath(externalModule)
			if err != nil {
				return fmt.Errorf(
					"docker SDK: escape external module path: %w", err,
				)
			}
			escapedVersion, err := module.EscapeVersion(externalVersion)
			if err != nil {
				return fmt.Errorf(
					"docker SDK: escape external module version: %w", err,
				)
			}

			expectedDir := "/gomodcache/" +
				escapedPath + "@" + escapedVersion
			expectedGoMod := "/gomodcache/cache/download/" +
				escapedPath + "/@v/" + escapedVersion + ".mod"

			if seenExternal ||
				item.Version != externalVersion ||
				item.Dir != expectedDir ||
				item.GoMod != expectedGoMod ||
				(len(item.Replace) != 0 &&
					string(item.Replace) != "null") {
				return fmt.Errorf(
					"docker SDK: invalid mixed v4 external module",
				)
			}

			seenExternal = true

		default:
			return fmt.Errorf(
				"docker SDK: unexpected mixed v4 module %q@%q",
				item.Path,
				item.Version,
			)
		}
	}

	if count != 3 ||
		!seenMain ||
		!seenLocal ||
		!seenExternal {
		return fmt.Errorf(
			"docker SDK: incomplete mixed v4 module graph",
		)
	}

	return nil
}

// rejectDuplicateMixedV4JSONKeys scans the full JSON stream
// before decoding into structs. encoding/json otherwise accepts
// duplicate object keys and silently overwrites earlier values.
//
// The depth limit also bounds recursive processing of untrusted
// JSON even when the total input size is within the graph limit.
func rejectDuplicateMixedV4JSONKeys(data []byte) error {
	const maxDepth = 32

	decoder := json.NewDecoder(bytes.NewReader(data))

	var readValue func(json.Token, int) error
	readValue = func(token json.Token, depth int) error {
		delim, isDelim := token.(json.Delim)
		if !isDelim {
			return nil
		}
		if depth >= maxDepth {
			return fmt.Errorf("mixed v4 JSON nesting limit exceeded")
		}

		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("mixed v4 JSON object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate mixed v4 JSON key %q", key)
				}
				seen[key] = struct{}{}

				valueToken, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := readValue(valueToken, depth+1); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil {
				return err
			}
			if closing != json.Delim('}') {
				return fmt.Errorf("invalid mixed v4 JSON object close")
			}
			return nil

		case '[':
			for decoder.More() {
				valueToken, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := readValue(valueToken, depth+1); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil {
				return err
			}
			if closing != json.Delim(']') {
				return fmt.Errorf("invalid mixed v4 JSON array close")
			}
			return nil

		default:
			return fmt.Errorf("unexpected mixed v4 JSON delimiter")
		}
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if token != json.Delim('{') {
			return fmt.Errorf("mixed v4 graph entry must be an object")
		}
		if err := readValue(token, 0); err != nil {
			return err
		}
	}
}
