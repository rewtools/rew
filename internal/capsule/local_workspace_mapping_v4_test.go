package capsule

import "testing"

func TestMapLocalWorkspaceV4(t *testing.T) {
	mainFiles := []WorkspaceFile{
		{
			Path: "go.mod",
			Data: []byte("module example.com/app\n\ngo 1.25\n"),
			Mode: 0600,
		},
		{
			Path: "main.go",
			Data: []byte("package main\n"),
			Mode: 0600,
		},
	}

	localFiles := []WorkspaceFile{
		{
			Path: "go.mod",
			Data: []byte("module example.com/lib\n\ngo 1.25\n"),
			Mode: 0600,
		},
		{
			Path: "lib.go",
			Data: []byte("package lib\n"),
			Mode: 0600,
		},
	}

	t.Run("maps two modules", func(t *testing.T) {
		got, err := MapLocalWorkspaceV4(
			"app", mainFiles,
			"lib", localFiles,
		)
		if err != nil {
			t.Fatal(err)
		}

		want := []string{
			"app/go.mod",
			"app/main.go",
			"lib/go.mod",
			"lib/lib.go",
		}

		if len(got) != len(want) {
			t.Fatalf("files=%d, want=%d", len(got), len(want))
		}

		for i := range want {
			if got[i].Path != want[i] {
				t.Fatalf(
					"file[%d]=%q, want=%q",
					i, got[i].Path, want[i],
				)
			}
		}
	})

	t.Run("rejects duplicate file", func(t *testing.T) {
		duplicate := append(
			append([]WorkspaceFile{}, localFiles...),
			localFiles[1],
		)

		if _, err := MapLocalWorkspaceV4(
			"app", mainFiles,
			"lib", duplicate,
		); err == nil {
			t.Fatal("expected duplicate rejection")
		}
	})

	t.Run("rejects missing local go.mod", func(t *testing.T) {
		if _, err := MapLocalWorkspaceV4(
			"app", mainFiles,
			"lib", localFiles[1:],
		); err == nil {
			t.Fatal("expected missing local go.mod rejection")
		}
	})

	t.Run("rejects overlapping roots", func(t *testing.T) {
		if _, err := MapLocalWorkspaceV4(
			"app", mainFiles,
			"app/sub", localFiles,
		); err == nil {
			t.Fatal("expected overlapping root rejection")
		}
	})

	t.Run("rejects traversal", func(t *testing.T) {
		bad := append([]WorkspaceFile{}, localFiles...)
		bad[1].Path = "../outside"

		if _, err := MapLocalWorkspaceV4(
			"app", mainFiles,
			"lib", bad,
		); err == nil {
			t.Fatal("expected path traversal rejection")
		}
	})
}

func TestMapLocalWorkspaceV4Security(t *testing.T) {
	mainFiles := []WorkspaceFile{
		{Path: "go.mod", Data: []byte("module example.com/app\n"), Mode: 0600},
	}
	localFiles := []WorkspaceFile{
		{Path: "go.mod", Data: []byte("module example.com/lib\n"), Mode: 0600},
	}

	t.Run("rejects file used as parent directory", func(t *testing.T) {
		files := append([]WorkspaceFile{}, mainFiles...)
		files = append(files,
			WorkspaceFile{Path: "config", Data: []byte("x"), Mode: 0600},
			WorkspaceFile{Path: "config/settings.yaml", Data: []byte("y"), Mode: 0600},
		)

		if _, err := MapLocalWorkspaceV4("app", files, "lib", localFiles); err == nil {
			t.Fatal("expected file/directory conflict rejection")
		}
	})

	t.Run("rejects parent directory added after child", func(t *testing.T) {
		files := append([]WorkspaceFile{}, mainFiles...)
		files = append(files,
			WorkspaceFile{Path: "config/settings.yaml", Data: []byte("y"), Mode: 0600},
			WorkspaceFile{Path: "config", Data: []byte("x"), Mode: 0600},
		)

		if _, err := MapLocalWorkspaceV4("app", files, "lib", localFiles); err == nil {
			t.Fatal("expected reverse-order path conflict rejection")
		}
	})

	t.Run("copies input bytes independently", func(t *testing.T) {
		input := append([]WorkspaceFile{}, mainFiles...)
		input = append(input, WorkspaceFile{
			Path: "main.go",
			Data: []byte("original"),
			Mode: 0600,
		})

		got, err := MapLocalWorkspaceV4("app", input, "lib", localFiles)
		if err != nil {
			t.Fatal(err)
		}

		input[1].Data[0] = 'X'

		if string(got[1].Data) != "original" {
			t.Fatalf("snapshot data changed: %q", got[1].Data)
		}
	})

	t.Run("rejects missing main go.mod", func(t *testing.T) {
		if _, err := MapLocalWorkspaceV4(
			"app", nil, "lib", localFiles,
		); err == nil {
			t.Fatal("expected missing main go.mod rejection")
		}
	})
}
