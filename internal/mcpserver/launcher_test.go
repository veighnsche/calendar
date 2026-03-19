package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCalendarAPIMCPBinarySupportsCommandTransport(t *testing.T) {
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "launcher-test-client", Version: "v1.0.0"}, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer upstream.Close()

	binary := buildTestBinary(t)
	cmd := exec.Command(binary,
		"--caldav-base-url", upstream.URL,
		"--caldav-username", "vince",
		"--caldav-password", "secret",
		"--calendar-default-name", "wall",
		"--api-bind-addr", "127.0.0.1:8090",
		"--default-timezone", "Europe/Amsterdam",
	)
	cmd.Dir = t.TempDir()
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect via binary: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if !hasTool(tools.Tools, "create_event") {
		t.Fatalf("expected create_event tool, got %#v", tools.Tools)
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()

	output := filepath.Join(t.TempDir(), "calendar-api-mcp")
	cmd := exec.Command("go", "build", "-o", output, "./cmd/calendar-api-mcp")
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build command-transport binary: %v\n%s", err, string(data))
	}
	return output
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("determine current file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}
