package replay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayV4FromFileRejectsNil(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *os.File, Options) (Result, error)
	}{
		{"local", RunV4WithOptionsFromFile},
		{"mixed", RunMixedV4WithOptionsFromFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.run(
				context.Background(), nil, Options{},
			)
			if err == nil {
				t.Fatal("nil descriptor accepted")
			}
			if !strings.Contains(err.Error(), "descriptor") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestReplayV4FromFileValidatesMemoryFirst(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *os.File, Options) (Result, error)
	}{
		{"local", RunV4WithOptionsFromFile},
		{"mixed", RunMixedV4WithOptionsFromFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.run(
				context.Background(),
				nil,
				Options{MemoryBytes: 12345},
			)
			if err == nil ||
				!strings.Contains(err.Error(), "memory") {
				t.Fatalf(
					"memory validation did not run first: %v",
					err,
				)
			}
		})
	}
}

func TestReplayV4FromFileDoesNotCloseCallerDescriptor(
	t *testing.T,
) {
	path := filepath.Join(t.TempDir(), "invalid.rew")
	if err := os.WriteFile(
		path, []byte(strings.Repeat("x", 2048)), 0600,
	); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		run  func(context.Context, *os.File, Options) (Result, error)
	}{
		{"local", RunV4WithOptionsFromFile},
		{"mixed", RunMixedV4WithOptionsFromFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			if _, err := tt.run(
				context.Background(), f, Options{},
			); err == nil {
				t.Fatal("invalid capsule accepted")
			}

			if _, err := f.Stat(); err != nil {
				t.Fatalf(
					"replay engine closed caller descriptor: %v",
					err,
				)
			}
		})
	}
}
