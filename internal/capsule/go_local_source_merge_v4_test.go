package capsule

import (
	"strconv"
	"testing"
)

func TestMergeLocalGoSourcesV4(t *testing.T) {
	t.Run("merges and sorts both modules", func(t *testing.T) {
		mainFiles, localFiles, err := MergeLocalGoSourcesV4(
			[]string{"main.go", "pkg/worker.go", "main.go"},
			[]string{"lib.go", "assets/message.txt", "lib.go"},
		)
		if err != nil {
			t.Fatal(err)
		}

		wantMain := []string{"go.mod", "main.go", "pkg/worker.go"}
		wantLocal := []string{"assets/message.txt", "go.mod", "lib.go"}

		check := func(got, want []string) {
			t.Helper()
			if len(got) != len(want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("got %v, want %v", got, want)
				}
			}
		}

		check(mainFiles, wantMain)
		check(localFiles, wantLocal)
	})

	t.Run("rejects traversal", func(t *testing.T) {
		_, _, err := MergeLocalGoSourcesV4(
			[]string{"main.go"},
			[]string{"../outside.go"},
		)
		if err == nil {
			t.Fatal("expected traversal rejection")
		}
	})

	t.Run("rejects noncanonical path", func(t *testing.T) {
		_, _, err := MergeLocalGoSourcesV4(
			[]string{"./main.go"},
			[]string{"lib.go"},
		)
		if err == nil {
			t.Fatal("expected noncanonical path rejection")
		}
	})

	t.Run("rejects absolute path", func(t *testing.T) {
		_, _, err := MergeLocalGoSourcesV4(
			[]string{"main.go"},
			[]string{"/tmp/lib.go"},
		)
		if err == nil {
			t.Fatal("expected absolute path rejection")
		}
	})

	t.Run("rejects combined count overflow", func(t *testing.T) {
		files := make([]string, MaxWorkspaceV3Files)
		for i := range files {
			files[i] = "main.go"
		}

		// Duplicates are intentionally collapsed, so this must not
		// trigger an overflow merely from input length.
		_, _, err := MergeLocalGoSourcesV4(
			files,
			[]string{"lib.go"},
		)
		if err != nil {
			t.Fatalf("duplicates should be deduplicated: %v", err)
		}
	})

	t.Run("rejects invalid separators", func(t *testing.T) {
		_, _, err := MergeLocalGoSourcesV4(
			[]string{"main.go"},
			[]string{`pkg\lib.go`},
		)
		if err == nil {
			t.Fatal("expected backslash rejection")
		}
	})

	t.Run("rejects empty source", func(t *testing.T) {
		_, _, err := MergeLocalGoSourcesV4(
			[]string{"main.go"},
			[]string{""},
		)
		if err == nil {
			t.Fatal("expected empty path rejection")
		}
	})

	t.Run("rejects actual unique overflow", func(t *testing.T) {
		// This verifies the limit using distinct canonical names.
		files := make([]string, MaxWorkspaceV3Files)
		for i := range files {
			files[i] = "file" + decimalLocalSourceIndexV4(i) + ".go"
		}

		_, _, err := MergeLocalGoSourcesV4(
			files, []string{"lib.go"},
		)
		if err == nil {
			t.Fatal("expected combined unique file limit rejection")
		}
	})
}

func decimalLocalSourceIndexV4(i int) string {
	return strconv.Itoa(i)
}
