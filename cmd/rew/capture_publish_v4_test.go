package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishCaptureV4(t *testing.T) {
	checkNoTemporaryFiles := func(t *testing.T, dir string) {
		t.Helper()

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range entries {
			if strings.HasPrefix(
				entry.Name(), ".rew-capture-",
			) {
				t.Fatalf(
					"temporary capsule remains: %s",
					entry.Name(),
				)
			}
		}
	}

	t.Run("successful publication", func(t *testing.T) {
		dir := t.TempDir()
		output := filepath.Join(dir, "failure.rew")
		expected := []byte("verified capsule bytes")

		err := publishCaptureV4(
			output,
			func(w io.Writer) error {
				_, err := w.Write(expected)
				return err
			},
		)
		if err != nil {
			t.Fatalf("publish capsule: %v", err)
		}

		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("published bytes differ: %q", got)
		}

		info, err := os.Lstat(output)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() ||
			info.Mode().Perm() != 0600 {
			t.Fatalf("unexpected output mode: %v", info.Mode())
		}

		checkNoTemporaryFiles(t, dir)
	})

	t.Run("partial writer failure", func(t *testing.T) {
		dir := t.TempDir()
		output := filepath.Join(dir, "failure.rew")

		failure := errors.New("intentional streaming failure")

		err := publishCaptureV4(
			output,
			func(w io.Writer) error {
				if _, err := w.Write(
					[]byte("partial capsule content"),
				); err != nil {
					return err
				}
				return failure
			},
		)

		if !errors.Is(err, failure) {
			t.Fatalf("wrong write failure: %v", err)
		}

		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Fatalf(
				"partial capsule published: %v", err,
			)
		}

		checkNoTemporaryFiles(t, dir)
	})

	t.Run("destination appears during write", func(t *testing.T) {
		dir := t.TempDir()
		output := filepath.Join(dir, "failure.rew")
		sentinel := []byte("OTHER PROCESS CONTENT")

		err := publishCaptureV4(
			output,
			func(w io.Writer) error {
				if _, err := w.Write(
					[]byte("capsule being written"),
				); err != nil {
					return err
				}

				// Deterministic race simulation:
				// another process creates the destination
				// after preflight but before publication.
				return os.WriteFile(
					output, sentinel, 0600,
				)
			},
		)

		if err == nil {
			t.Fatal("concurrent destination accepted")
		}

		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, sentinel) {
			t.Fatalf(
				"concurrent destination overwritten: %q",
				got,
			)
		}

		checkNoTemporaryFiles(t, dir)
	})

	t.Run("failure before first byte", func(t *testing.T) {
		dir := t.TempDir()
		output := filepath.Join(dir, "failure.rew")
		failure := errors.New("pre-write validation failure")

		calls := 0
		err := publishCaptureV4(
			output,
			func(w io.Writer) error {
				calls++
				return failure
			},
		)

		if calls != 1 || !errors.Is(err, failure) {
			t.Fatalf(
				"unexpected pre-write result: calls=%d err=%v",
				calls, err,
			)
		}
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Fatalf("pre-write failure published output: %v", err)
		}
		checkNoTemporaryFiles(t, dir)
	})

	t.Run("writer failure preserves concurrent destination", func(t *testing.T) {
		dir := t.TempDir()
		output := filepath.Join(dir, "failure.rew")
		sentinel := []byte("CONCURRENT OWNER CONTENT")
		failure := errors.New("injected failure after partial write")

		err := publishCaptureV4(
			output,
			func(w io.Writer) error {
				if _, err := w.Write(
					[]byte("incomplete capsule"),
				); err != nil {
					return err
				}

				// Another actor creates the final path while
				// our temporary stream is still in progress.
				f, err := os.OpenFile(
					output,
					os.O_WRONLY|os.O_CREATE|os.O_EXCL,
					0600,
				)
				if err != nil {
					return err
				}
				_, writeErr := f.Write(sentinel)
				closeErr := f.Close()
				if writeErr != nil {
					return writeErr
				}
				if closeErr != nil {
					return closeErr
				}

				return failure
			},
		)

		if !errors.Is(err, failure) {
			t.Fatalf("incorrect writer failure: %v", err)
		}

		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatalf("concurrent output was removed: %v", err)
		}
		if !bytes.Equal(got, sentinel) {
			t.Fatalf("concurrent output was modified: %q", got)
		}

		checkNoTemporaryFiles(t, dir)
	})

	t.Run("existing destination rejected", func(t *testing.T) {
		dir := t.TempDir()
		output := filepath.Join(dir, "failure.rew")
		sentinel := []byte("EXISTING FILE")

		if err := os.WriteFile(
			output, sentinel, 0600,
		); err != nil {
			t.Fatal(err)
		}

		calls := 0
		err := publishCaptureV4(
			output,
			func(w io.Writer) error {
				calls++
				return nil
			},
		)

		if err == nil || calls != 0 {
			t.Fatalf(
				"existing destination not rejected: err=%v calls=%d",
				err, calls,
			)
		}

		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, sentinel) {
			t.Fatalf("existing output changed: %q", got)
		}

		checkNoTemporaryFiles(t, dir)
	})
}
