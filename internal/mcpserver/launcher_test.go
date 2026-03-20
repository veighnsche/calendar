package mcpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		switch {
		case r.URL.Path == "/" && strings.Contains(string(body), "current-user-principal"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<multistatus xmlns="DAV:">
  <response>
    <href>/</href>
    <propstat>
      <prop>
        <current-user-principal>
          <href>/principals/vince/</href>
        </current-user-principal>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`))
		case r.URL.Path == "/principals/vince/" && strings.Contains(string(body), "calendar-home-set"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <response>
    <href>/principals/vince/</href>
    <propstat>
      <prop>
        <c:calendar-home-set>
          <href>/calendars/</href>
        </c:calendar-home-set>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`))
		case r.URL.Path == "/calendars/":
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <response>
    <href>/calendars/</href>
    <propstat>
      <prop>
        <displayname>Calendars</displayname>
        <resourcetype>
          <collection />
        </resourcetype>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
  <response>
    <href>/calendars/wall/</href>
    <propstat>
      <prop>
        <displayname>Wall</displayname>
        <resourcetype>
          <collection />
          <c:calendar />
        </resourcetype>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
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
