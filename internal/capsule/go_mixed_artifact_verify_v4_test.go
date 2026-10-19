package capsule

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

func TestVerifyMixedGoModuleArtifactsV4(t *testing.T) {
	fixture := makeVerifiedSnapshotFixture(t)

	if len(fixture.Manifest.GoModules) != 1 {
		t.Fatalf(
			"expected one external fixture module, got %d",
			len(fixture.Manifest.GoModules),
		)
	}

	external := fixture.Manifest.GoModules[0]
	goSum := fixtureGoSumV3(t, fixture)

	local := GoModuleIdentityV4{
		Path:    "example.com/local-lib",
		Version: "v0.0.0",
		Replace: &GoModuleReplacementV4{
			Path:         "../lib",
			SnapshotPath: "lib",
		},
	}

	selected := []GoModuleIdentityV4{
		local,
		{
			Path:    external.Path,
			Version: external.Version,
		},
	}

	sort.Slice(selected, func(i, j int) bool {
		return selected[i].Path < selected[j].Path
	})

	t.Run("valid verified external module", func(t *testing.T) {
		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		if err := VerifyMixedGoModuleArtifactsV4(
			selected,
			[]GoModuleV3{external},
			goSum,
			store,
		); err != nil {
			t.Fatalf("valid mixed artifacts rejected: %v", err)
		}
	})

	t.Run("missing external metadata", func(t *testing.T) {
		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		if err := VerifyMixedGoModuleArtifactsV4(
			selected,
			nil,
			goSum,
			store,
		); err == nil {
			t.Fatal("missing external module accepted")
		}
	})

	t.Run("wrong external version", func(t *testing.T) {
		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		changed := external
		changed.Version = "v99.0.0"

		if err := VerifyMixedGoModuleArtifactsV4(
			selected,
			[]GoModuleV3{changed},
			goSum,
			store,
		); err == nil {
			t.Fatal("wrong module version accepted")
		}
	})

	t.Run("incorrect go.sum", func(t *testing.T) {
		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		if err := VerifyMixedGoModuleArtifactsV4(
			selected,
			[]GoModuleV3{external},
			[]byte("invalid go.sum\n"),
			store,
		); err == nil {
			t.Fatal("invalid go.sum accepted")
		}
	})

	t.Run("missing artifact store", func(t *testing.T) {
		if err := VerifyMixedGoModuleArtifactsV4(
			selected,
			[]GoModuleV3{external},
			goSum,
			nil,
		); err == nil {
			t.Fatal("nil artifact store accepted")
		}
	})

	t.Run("incorrect artifact checksum", func(t *testing.T) {
		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		changed := external
		changed.Artifacts = append(
			[]GoModuleArtifact(nil),
			external.Artifacts...,
		)
		changed.Artifacts[0].SHA256 =
			"sha256:0000000000000000000000000000000000000000000000000000000000000000"

		if err := VerifyMixedGoModuleArtifactsV4(
			selected,
			[]GoModuleV3{changed},
			goSum,
			store,
		); err == nil {
			t.Fatal("incorrect artifact checksum accepted")
		}
	})

	t.Run("missing ZIP artifact", func(t *testing.T) {
		store, err := NewArtifactStoreV3(MaxGoModuleTotalSizeV3)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })

		for _, metadata := range external.Artifacts {
			if strings.HasSuffix(metadata.Path, ".zip") {
				continue
			}

			data := fixture.Artifacts[metadata.Path]

			if err := store.Put(
				metadata.Path,
				bytes.NewReader(data),
				int64(len(data)),
				strings.TrimPrefix(metadata.SHA256, "sha256:"),
			); err != nil {
				t.Fatal(err)
			}
		}

		if err := VerifyMixedGoModuleArtifactsV4(
			selected, []GoModuleV3{external}, goSum, store,
		); err == nil {
			t.Fatal("missing ZIP artifact accepted")
		}
	})

	t.Run("unexpected stored artifact", func(t *testing.T) {
		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		data := []byte("unexpected artifact")

		if err := store.Put(
			"unexpected.txt",
			bytes.NewReader(data),
			int64(len(data)),
			strings.TrimPrefix(SHA256(data), "sha256:"),
		); err != nil {
			t.Fatal(err)
		}

		if err := VerifyMixedGoModuleArtifactsV4(
			selected, []GoModuleV3{external}, goSum, store,
		); err == nil {
			t.Fatal("unexpected stored artifact accepted")
		}
	})

	t.Run("corrupted ZIP with matching manifest SHA256", func(t *testing.T) {
		changed := external
		changed.Artifacts = append(
			[]GoModuleArtifact(nil),
			external.Artifacts...,
		)

		artifacts := make(
			map[string][]byte,
			len(fixture.Artifacts),
		)

		for name, data := range fixture.Artifacts {
			artifacts[name] = append([]byte(nil), data...)
		}

		foundZIP := false

		for index := range changed.Artifacts {
			metadata := &changed.Artifacts[index]

			if !strings.HasSuffix(metadata.Path, ".zip") {
				continue
			}

			data := []byte("not a valid ZIP archive")
			artifacts[metadata.Path] = data
			metadata.Size = int64(len(data))
			metadata.SHA256 = SHA256(data)
			foundZIP = true
		}

		if !foundZIP {
			t.Fatal("fixture missing ZIP metadata")
		}

		store := storeModuleFixtureV3(
			t, changed, artifacts,
		)

		if err := VerifyMixedGoModuleArtifactsV4(
			selected, []GoModuleV3{changed}, goSum, store,
		); err == nil {
			t.Fatal("corrupted ZIP accepted")
		}
	})

	t.Run("missing info metadata", func(t *testing.T) {
		changed := external
		changed.Artifacts = nil

		for _, metadata := range external.Artifacts {
			if strings.HasSuffix(metadata.Path, ".info") {
				continue
			}
			changed.Artifacts = append(
				changed.Artifacts, metadata,
			)
		}

		store := storeModuleFixtureV3(
			t, external, fixture.Artifacts,
		)

		if err := VerifyMixedGoModuleArtifactsV4(
			selected, []GoModuleV3{changed}, goSum, store,
		); err == nil {
			t.Fatal("incomplete artifact metadata accepted")
		}
	})

}
