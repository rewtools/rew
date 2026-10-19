package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/rewtools/rew/internal/capsule"
)

type replayV4Kind uint8

const (
	replayV4Local replayV4Kind = iota + 1
	replayV4Mixed
)

// inspectReplayV4Kind reads only bounded routing metadata.
//
// This is NOT capsule verification. The selected verified reader
// must independently verify the complete archive before Docker
// can receive any capsule content.
func inspectReplayV4Kind(path string) (replayV4Kind, error) {
	f, err := os.OpenFile(
		path, os.O_RDONLY|syscall.O_NOFOLLOW, 0,
	)
	if err != nil {
		return 0, fmt.Errorf("open v4 capsule: %w", err)
	}
	defer f.Close()

	return inspectReplayV4KindFromFile(f)
}

// inspectReplayV4KindFromFile classifies a capsule using the
// caller-owned descriptor. It never opens the pathname or
// verifies the capsule's complete contents.
func inspectReplayV4KindFromFile(
	f *os.File,
) (replayV4Kind, error) {
	if f == nil {
		return 0, fmt.Errorf("nil v4 capsule descriptor")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek v4 capsule: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat v4 capsule: %w", err)
	}

	if !info.Mode().IsRegular() ||
		info.Size() < 1024 ||
		info.Size() > capsule.MaxCapsuleSizeV3 {
		return 0, fmt.Errorf("invalid v4 capsule file or size")
	}

	tr := tar.NewReader(
		io.LimitReader(f, capsule.MaxCapsuleSizeV3+1),
	)

	h, err := tr.Next()
	if err != nil {
		return 0, fmt.Errorf("read v4 manifest header: %w", err)
	}

	if h.Name != "manifest.json" ||
		h.Typeflag != tar.TypeReg ||
		h.Size <= 0 ||
		h.Size > capsule.MaxManifestSize {
		return 0, fmt.Errorf("invalid v4 manifest header")
	}

	data, err := io.ReadAll(
		io.LimitReader(tr, capsule.MaxManifestSize+1),
	)
	if err != nil {
		return 0, fmt.Errorf("read v4 routing manifest: %w", err)
	}

	if int64(len(data)) != h.Size {
		return 0, fmt.Errorf("v4 routing manifest size mismatch")
	}

	var manifest capsule.ManifestV4
	if err := json.Unmarshal(data, &manifest); err != nil {
		return 0, fmt.Errorf("decode v4 routing manifest: %w", err)
	}

	if manifest.SchemaVersion != capsule.SchemaVersionV4 {
		return 0, fmt.Errorf("unexpected v4 schema version")
	}

	switch {
	case len(manifest.GoModules) == 0 &&
		len(manifest.GoSelectedModules) == 1:
		return replayV4Local, nil

	case len(manifest.GoModules) == 1 &&
		len(manifest.GoSelectedModules) == 2:
		return replayV4Mixed, nil

	default:
		return 0, fmt.Errorf("unsupported v4 replay module layout")
	}
}
