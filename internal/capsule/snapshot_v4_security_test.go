package capsule

import (
	"bytes"
	"errors"
	"testing"
)

func TestWriteWorkspaceSnapshotV4RejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*VerifiedCapsuleV4)
	}{
		{
			name: "tampered source bytes",
			mutate: func(v *VerifiedCapsuleV4) {
				v.Files[1].Data = []byte("modified\n")
			},
		},
		{
			name: "missing workspace file",
			mutate: func(v *VerifiedCapsuleV4) {
				v.Files = v.Files[:len(v.Files)-1]
			},
		},
		{
			name: "unexpected workspace path",
			mutate: func(v *VerifiedCapsuleV4) {
				v.Files[1].Path = "outside.go"
			},
		},
		{
			name: "invalid schema",
			mutate: func(v *VerifiedCapsuleV4) {
				v.Manifest.SchemaVersion = 3
			},
		},
		{
			name: "corrupted metadata hash",
			mutate: func(v *VerifiedCapsuleV4) {
				v.Manifest.Workspace.Files[0].SHA256 =
					SHA256([]byte("incorrect data"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verified := verifiedRestoreFixtureV4(t)
			tc.mutate(&verified)

			var output bytes.Buffer
			if err := WriteWorkspaceSnapshotV4(
				&output, verified,
			); err == nil {
				t.Fatal("accepted invalid V4 snapshot input")
			}

			if output.Len() != 0 {
				t.Fatalf(
					"invalid input emitted %d TAR bytes",
					output.Len(),
				)
			}
		})
	}
}

type failingSnapshotWriterV4 struct{}

func (failingSnapshotWriterV4) Write([]byte) (int, error) {
	return 0, errors.New("simulated output failure")
}

func TestWriteWorkspaceSnapshotV4PropagatesOutputFailure(t *testing.T) {
	verified := verifiedRestoreFixtureV4(t)

	err := WriteWorkspaceSnapshotV4(
		failingSnapshotWriterV4{},
		verified,
	)
	if err == nil {
		t.Fatal("snapshot writer ignored output failure")
	}
}

func TestWriteWorkspaceSnapshotV4RejectsNilWriter(t *testing.T) {
	verified := verifiedRestoreFixtureV4(t)

	if err := WriteWorkspaceSnapshotV4(
		nil, verified,
	); err == nil {
		t.Fatal("accepted nil snapshot writer")
	}
}
