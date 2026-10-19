package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func TestDockerExecWorkingDirectoryAPIv4(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		legacy    bool
		want      string
		wantCalls int
	}{
		{
			name:      "V4 main directory",
			dir:       "/workspace/app",
			want:      "/workspace/app",
			wantCalls: 1,
		},
		{
			name:      "V3 legacy wrapper",
			legacy:    true,
			want:      "/workspace",
			wantCalls: 1,
		},
		{
			name:      "reject sibling directory",
			dir:       "/workspace/lib",
			wantCalls: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var capturedDir string

			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					calls++

					if r.Method != http.MethodPost ||
						!strings.HasSuffix(r.URL.Path, "/exec") {
						t.Errorf(
							"unexpected Docker request: %s %s",
							r.Method, r.URL.Path,
						)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					var request struct {
						WorkingDir string `json:"WorkingDir"`
					}

					if err := json.NewDecoder(r.Body).Decode(
						&request,
					); err != nil {
						t.Errorf("decode ExecCreate: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}

					capturedDir = request.WorkingDir

					// Deliberately stop execution after recording
					// the Docker ExecCreate request.
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(
						`{"message":"expected test stop"}`,
					))
				},
			))
			defer server.Close()

			cli, err := client.NewClientWithOpts(
				client.WithHost(server.URL),
				client.WithVersion("1.44"),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()

			ctx := context.Background()
			var execErr error

			if tc.legacy {
				_, _, _, execErr = executeInContainer(
					ctx,
					cli,
					"test-container",
					"10001:10001",
					[]string{"go", "version"},
				)
			} else {
				_, _, _, execErr = executeInContainerAtDirectory(
					ctx,
					cli,
					"test-container",
					"10001:10001",
					tc.dir,
					[]string{"go", "version"},
				)
			}

			if execErr == nil {
				t.Fatal("expected test execution to stop")
			}

			if calls != tc.wantCalls {
				t.Fatalf(
					"Docker calls=%d, want=%d",
					calls, tc.wantCalls,
				)
			}

			if tc.wantCalls > 0 && capturedDir != tc.want {
				t.Fatalf(
					"WorkingDir=%q, want=%q",
					capturedDir, tc.want,
				)
			}
		})
	}
}
