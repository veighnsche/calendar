package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"calendar-api/internal/config"
	"calendar-api/internal/events"
	"calendar-api/internal/service"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLiveHTTPFlow(t *testing.T) {
	cfg := requireLiveConfig(t)
	ensureCalendarExists(t, cfg)

	apiURL, stop := startHTTPBinary(t, cfg)
	defer stop()

	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	end := start.Add(45 * time.Minute)

	healthResp := mustRequest(t, http.MethodGet, apiURL+"/healthz", nil, nil)
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", healthResp.StatusCode)
	}

	var healthBody struct {
		Status string `json:"status"`
	}
	decodeBody(t, healthResp, &healthBody)
	if healthBody.Status != "ok" {
		t.Fatalf("unexpected health body: %#v", healthBody)
	}

	calsResp := mustRequest(t, http.MethodGet, apiURL+"/calendars", nil, nil)
	if calsResp.StatusCode != http.StatusOK {
		t.Fatalf("calendars status = %d", calsResp.StatusCode)
	}

	var calsBody struct {
		Calendars []events.Calendar `json:"calendars"`
	}
	decodeBody(t, calsResp, &calsBody)
	if !containsCalendar(calsBody.Calendars, cfg.DefaultCalendar) {
		t.Fatalf("calendar %q not listed in %#v", cfg.DefaultCalendar, calsBody.Calendars)
	}

	title := "calendar-api live http " + time.Now().UTC().Format("20060102-150405")
	createReq := events.CreateRequest{
		Calendar: cfg.DefaultCalendar,
		Title:    title,
		Start:    start.Format(time.RFC3339),
		End:      end.Format(time.RFC3339),
		Timezone: "UTC",
	}

	createResp := mustRequest(t, http.MethodPost, apiURL+"/events", createReq, map[string]string{
		"Content-Type": "application/json",
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}

	var createBody struct {
		Event events.Event `json:"event"`
	}
	decodeBody(t, createResp, &createBody)
	if createBody.Event.ID == "" || createBody.Event.ETag == "" {
		t.Fatalf("create returned incomplete event: %#v", createBody.Event)
	}

	currentID := createBody.Event.ID
	currentETag := createBody.Event.ETag
	currentCalendar := createBody.Event.Calendar

	defer func() {
		if currentID == "" || currentETag == "" {
			return
		}
		_ = deleteViaHTTP(apiURL, currentCalendar, currentID, currentETag)
	}()

	assertCalDAVObjectExists(t, cfg, currentCalendar, currentID)

	listURL := fmt.Sprintf("%s/events?calendar=%s&from=%s&to=%s", apiURL, currentCalendar, start.Add(-time.Hour).Format(time.RFC3339), end.Add(time.Hour).Format(time.RFC3339))
	listResp := mustRequest(t, http.MethodGet, listURL, nil, nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listResp.StatusCode)
	}

	var listBody struct {
		Events []events.Event `json:"events"`
	}
	decodeBody(t, listResp, &listBody)
	if !containsEvent(listBody.Events, currentID) {
		t.Fatalf("expected created event %q in %#v", currentID, listBody.Events)
	}

	getResp := mustRequest(t, http.MethodGet, fmt.Sprintf("%s/events/%s?calendar=%s", apiURL, currentID, currentCalendar), nil, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", getResp.StatusCode)
	}

	var getBody struct {
		Event events.Event `json:"event"`
	}
	decodeBody(t, getResp, &getBody)
	if getBody.Event.Title != title {
		t.Fatalf("unexpected get body: %#v", getBody.Event)
	}

	updatedTitle := title + " updated"
	patchReq := map[string]any{
		"title": updatedTitle,
	}
	patchResp := mustRequest(t, http.MethodPatch, fmt.Sprintf("%s/events/%s?calendar=%s", apiURL, currentID, currentCalendar), patchReq, map[string]string{
		"Content-Type": "application/json",
		"If-Match":     currentETag,
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d", patchResp.StatusCode)
	}

	var patchBody struct {
		Event events.Event `json:"event"`
	}
	decodeBody(t, patchResp, &patchBody)
	if patchBody.Event.Title != updatedTitle {
		t.Fatalf("unexpected patch body: %#v", patchBody.Event)
	}
	currentETag = patchBody.Event.ETag

	movedStart := start.Add(2 * time.Hour)
	movedEnd := end.Add(2 * time.Hour)
	moveReq := events.MoveRequest{
		Start: movedStart.Format(time.RFC3339),
		End:   movedEnd.Format(time.RFC3339),
	}
	moveResp := mustRequest(t, http.MethodPost, fmt.Sprintf("%s/events/%s/move?calendar=%s", apiURL, currentID, currentCalendar), moveReq, map[string]string{
		"Content-Type": "application/json",
		"If-Match":     currentETag,
	})
	if moveResp.StatusCode != http.StatusOK {
		t.Fatalf("move status = %d", moveResp.StatusCode)
	}

	var moveBody struct {
		Event events.Event `json:"event"`
	}
	decodeBody(t, moveResp, &moveBody)
	if !moveBody.Event.Start.Equal(movedStart) || !moveBody.Event.End.Equal(movedEnd) {
		t.Fatalf("unexpected moved event: %#v", moveBody.Event)
	}
	currentETag = moveBody.Event.ETag

	availabilityURL := fmt.Sprintf(
		"%s/availability?calendar=%s&from=%s&to=%s&duration_minutes=30",
		apiURL,
		currentCalendar,
		movedStart.Add(-time.Hour).Format(time.RFC3339),
		movedEnd.Add(time.Hour).Format(time.RFC3339),
	)
	availabilityResp := mustRequest(t, http.MethodGet, availabilityURL, nil, nil)
	if availabilityResp.StatusCode != http.StatusOK {
		t.Fatalf("availability status = %d", availabilityResp.StatusCode)
	}

	var availabilityBody struct {
		Busy []events.Interval `json:"busy"`
	}
	decodeBody(t, availabilityResp, &availabilityBody)
	if !containsBusyInterval(availabilityBody.Busy, movedStart, movedEnd) {
		t.Fatalf("expected moved interval in busy list: %#v", availabilityBody.Busy)
	}

	deleteResp := mustRequest(t, http.MethodDelete, fmt.Sprintf("%s/events/%s?calendar=%s&etag=%s", apiURL, currentID, currentCalendar, currentETag), nil, nil)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", deleteResp.StatusCode)
	}

	var deleteBody struct {
		Deleted bool `json:"deleted"`
	}
	decodeBody(t, deleteResp, &deleteBody)
	if !deleteBody.Deleted {
		t.Fatalf("unexpected delete body: %#v", deleteBody)
	}
	currentETag = ""

	assertCalDAVObjectMissing(t, cfg, currentCalendar, currentID)

	missingResp := mustRequest(t, http.MethodGet, fmt.Sprintf("%s/events/%s?calendar=%s", apiURL, createBody.Event.ID, currentCalendar), nil, nil)
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", missingResp.StatusCode)
	}
}

func TestLiveMCPFlow(t *testing.T) {
	ctx := context.Background()
	cfg := requireLiveConfig(t)
	ensureCalendarExists(t, cfg)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "live-e2e-client", Version: "v1.0.0"}, nil)
	cmd := exec.Command(buildBinary(t, "./cmd/calendar-api-mcp"))
	cmd.Dir = t.TempDir()
	cmd.Args = append(cmd.Args, runtimeArgs(cfg, "127.0.0.1:8090")...)
	cmd.Env = minimalEnv()

	clientSession, err := mcpClient.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	calResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "list_calendars"})
	if err != nil {
		t.Fatalf("list_calendars: %v", err)
	}
	if calResult.IsError {
		t.Fatalf("list_calendars tool error: %#v", calResult.Content)
	}

	var calBody struct {
		Calendars []events.Calendar `json:"calendars"`
	}
	decodeStructured(t, calResult.StructuredContent, &calBody)
	if !containsCalendar(calBody.Calendars, cfg.DefaultCalendar) {
		t.Fatalf("calendar %q not listed in %#v", cfg.DefaultCalendar, calBody.Calendars)
	}

	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
	end := start.Add(30 * time.Minute)
	title := "calendar-api live mcp " + time.Now().UTC().Format("20060102-150405")

	createResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_event",
		Arguments: map[string]any{
			"calendar": cfg.DefaultCalendar,
			"title":    title,
			"start":    start.Format(time.RFC3339),
			"end":      end.Format(time.RFC3339),
			"timezone": "UTC",
		},
	})
	if err != nil {
		t.Fatalf("create_event: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("create_event tool error: %#v", createResult.Content)
	}

	var createBody service.EventResult
	decodeStructured(t, createResult.StructuredContent, &createBody)
	if createBody.Event.ID == "" || createBody.Event.ETag == "" {
		t.Fatalf("unexpected create body: %#v", createBody)
	}

	currentID := createBody.Event.ID
	currentETag := createBody.Event.ETag
	currentCalendar := createBody.Event.Calendar

	defer func() {
		if currentID == "" || currentETag == "" {
			return
		}
		_, _ = clientSession.CallTool(ctx, &mcp.CallToolParams{
			Name: "delete_event",
			Arguments: map[string]any{
				"calendar": currentCalendar,
				"id":       currentID,
				"etag":     currentETag,
			},
		})
	}()

	assertCalDAVObjectExists(t, cfg, currentCalendar, currentID)

	listResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "list_events",
		Arguments: map[string]any{
			"calendar": currentCalendar,
			"from":     start.Add(-time.Hour).Format(time.RFC3339),
			"to":       end.Add(time.Hour).Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("list_events: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list_events tool error: %#v", listResult.Content)
	}

	var listBody struct {
		Events []events.Event `json:"events"`
	}
	decodeStructured(t, listResult.StructuredContent, &listBody)
	if !containsEvent(listBody.Events, currentID) {
		t.Fatalf("expected created event %q in %#v", currentID, listBody.Events)
	}

	getResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_event",
		Arguments: map[string]any{
			"calendar": currentCalendar,
			"id":       currentID,
		},
	})
	if err != nil {
		t.Fatalf("get_event: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("get_event tool error: %#v", getResult.Content)
	}

	var getBody struct {
		Event events.Event `json:"event"`
	}
	decodeStructured(t, getResult.StructuredContent, &getBody)
	if getBody.Event.Title != title {
		t.Fatalf("unexpected get_event body: %#v", getBody.Event)
	}

	updatedTitle := title + " updated"
	updateResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_event",
		Arguments: map[string]any{
			"calendar": currentCalendar,
			"id":       currentID,
			"title":    updatedTitle,
			"etag":     currentETag,
		},
	})
	if err != nil {
		t.Fatalf("update_event: %v", err)
	}
	if updateResult.IsError {
		t.Fatalf("update_event tool error: %#v", updateResult.Content)
	}

	var updateBody service.EventResult
	decodeStructured(t, updateResult.StructuredContent, &updateBody)
	if updateBody.Event.Title != updatedTitle {
		t.Fatalf("unexpected update_event body: %#v", updateBody.Event)
	}
	currentETag = updateBody.Event.ETag

	movedStart := start.Add(90 * time.Minute)
	movedEnd := end.Add(90 * time.Minute)
	moveResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "move_event",
		Arguments: map[string]any{
			"calendar": currentCalendar,
			"id":       currentID,
			"start":    movedStart.Format(time.RFC3339),
			"end":      movedEnd.Format(time.RFC3339),
			"etag":     currentETag,
		},
	})
	if err != nil {
		t.Fatalf("move_event: %v", err)
	}
	if moveResult.IsError {
		t.Fatalf("move_event tool error: %#v", moveResult.Content)
	}

	var moveBody service.EventResult
	decodeStructured(t, moveResult.StructuredContent, &moveBody)
	if !moveBody.Event.Start.Equal(movedStart) || !moveBody.Event.End.Equal(movedEnd) {
		t.Fatalf("unexpected move_event body: %#v", moveBody.Event)
	}
	currentETag = moveBody.Event.ETag

	availabilityResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_availability",
		Arguments: map[string]any{
			"calendar":        currentCalendar,
			"from":            movedStart.Add(-time.Hour).Format(time.RFC3339),
			"to":              movedEnd.Add(time.Hour).Format(time.RFC3339),
			"durationMinutes": 30,
		},
	})
	if err != nil {
		t.Fatalf("get_availability: %v", err)
	}
	if availabilityResult.IsError {
		t.Fatalf("get_availability tool error: %#v", availabilityResult.Content)
	}

	var availabilityBody struct {
		Busy []events.Interval `json:"busy"`
	}
	decodeStructured(t, availabilityResult.StructuredContent, &availabilityBody)
	if !containsBusyInterval(availabilityBody.Busy, movedStart, movedEnd) {
		t.Fatalf("expected moved interval in busy list: %#v", availabilityBody.Busy)
	}

	deleteResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "delete_event",
		Arguments: map[string]any{
			"calendar": currentCalendar,
			"id":       currentID,
			"etag":     currentETag,
		},
	})
	if err != nil {
		t.Fatalf("delete_event: %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("delete_event tool error: %#v", deleteResult.Content)
	}

	var deleteBody struct {
		Deleted bool `json:"deleted"`
	}
	decodeStructured(t, deleteResult.StructuredContent, &deleteBody)
	if !deleteBody.Deleted {
		t.Fatalf("unexpected delete_event body: %#v", deleteBody)
	}
	currentETag = ""

	assertCalDAVObjectMissing(t, cfg, currentCalendar, currentID)
}

func requireLiveConfig(t *testing.T) config.Config {
	t.Helper()

	if os.Getenv("CALENDAR_API_LIVE_E2E") != "1" {
		t.Skip("set CALENDAR_API_LIVE_E2E=1 to run live CalDAV end-to-end tests")
	}

	cfg := config.Config{
		CalDAVBaseURL:   os.Getenv("CALDAV_BASE_URL"),
		CalDAVUsername:  os.Getenv("CALDAV_USERNAME"),
		CalDAVPassword:  os.Getenv("CALDAV_PASSWORD"),
		DefaultCalendar: firstNonEmpty(os.Getenv("CALENDAR_E2E_CALENDAR"), os.Getenv("CALENDAR_DEFAULT_NAME"), "calendar-api-test"),
		BindAddr:        firstNonEmpty(os.Getenv("API_BIND_ADDR"), "127.0.0.1:8090"),
		DefaultTimezone: firstNonEmpty(os.Getenv("DEFAULT_TIMEZONE"), os.Getenv("TZ"), "Europe/Amsterdam"),
	}

	missing := make([]string, 0, 3)
	if cfg.CalDAVBaseURL == "" {
		missing = append(missing, "CALDAV_BASE_URL")
	}
	if cfg.CalDAVUsername == "" {
		missing = append(missing, "CALDAV_USERNAME")
	}
	if cfg.CalDAVPassword == "" {
		missing = append(missing, "CALDAV_PASSWORD")
	}
	if len(missing) > 0 {
		t.Fatalf("missing live E2E config: %s", strings.Join(missing, ", "))
	}
	return cfg
}

func ensureCalendarExists(t *testing.T, cfg config.Config) {
	t.Helper()

	if _, ok := discoverCalendarCollectionURL(t, cfg, cfg.DefaultCalendar); ok {
		return
	}

	homeSetURL := discoverCalendarHomeSetURL(t, cfg)
	req, err := http.NewRequest("MKCALENDAR", joinCollectionURL(t, homeSetURL, cfg.DefaultCalendar), strings.NewReader(fmt.Sprintf(`<C:mkcalendar xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:set><D:prop><D:displayname>%s</D:displayname><C:supported-calendar-component-set><C:comp name="VEVENT"/></C:supported-calendar-component-set></D:prop></D:set></C:mkcalendar>`, cfg.DefaultCalendar)))
	if err != nil {
		t.Fatalf("new MKCALENDAR request: %v", err)
	}
	req.SetBasicAuth(cfg.CalDAVUsername, cfg.CalDAVPassword)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("ensure calendar exists: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusMethodNotAllowed, http.StatusConflict:
	default:
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected MKCALENDAR status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func startHTTPBinary(t *testing.T, cfg config.Config) (string, func()) {
	t.Helper()

	binary := buildBinary(t, "./cmd/calendar-api")
	bindAddr := freeBindAddr(t)

	cmd := exec.Command(binary, runtimeArgs(cfg, bindAddr)...)
	cmd.Dir = t.TempDir()
	cmd.Env = minimalEnv()

	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start calendar-api: %v", err)
	}

	stop := func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		case <-done:
		}
	}

	url := "http://" + bindAddr
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusServiceUnavailable {
				return url, stop
			}
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Fatalf("calendar-api exited early: %s", logBuf.String())
		}
		time.Sleep(200 * time.Millisecond)
	}

	stop()
	t.Fatalf("calendar-api did not become ready: %s", logBuf.String())
	return "", nil
}

func buildBinary(t *testing.T, pkg string) string {
	t.Helper()

	output := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Dir = projectRoot(t)
	cmd.Env = os.Environ()
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, strings.TrimSpace(string(data)))
	}
	return output
}

func runtimeArgs(cfg config.Config, bindAddr string) []string {
	return []string{
		"--caldav-base-url", cfg.CalDAVBaseURL,
		"--caldav-username", cfg.CalDAVUsername,
		"--caldav-password", cfg.CalDAVPassword,
		"--calendar-default-name", cfg.DefaultCalendar,
		"--api-bind-addr", bindAddr,
		"--default-timezone", cfg.DefaultTimezone,
	}
}

func minimalEnv() []string {
	return []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("determine current file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func freeBindAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate bind addr: %v", err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func assertCalDAVObjectExists(t *testing.T, cfg config.Config, calendar, id string) {
	t.Helper()

	collectionURL, ok := discoverCalendarCollectionURL(t, cfg, calendar)
	if !ok {
		t.Fatalf("calendar %q not found", calendar)
	}

	req, err := http.NewRequest(http.MethodGet, joinCollectionURL(t, collectionURL, id+".ics"), nil)
	if err != nil {
		t.Fatalf("new caldav GET request: %v", err)
	}
	req.SetBasicAuth(cfg.CalDAVUsername, cfg.CalDAVPassword)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch caldav object: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected caldav object %s/%s, got %d: %s", calendar, id, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func assertCalDAVObjectMissing(t *testing.T, cfg config.Config, calendar, id string) {
	t.Helper()

	collectionURL, ok := discoverCalendarCollectionURL(t, cfg, calendar)
	if !ok {
		return
	}

	req, err := http.NewRequest(http.MethodGet, joinCollectionURL(t, collectionURL, id+".ics"), nil)
	if err != nil {
		t.Fatalf("new caldav GET request: %v", err)
	}
	req.SetBasicAuth(cfg.CalDAVUsername, cfg.CalDAVPassword)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch caldav object: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected caldav object %s/%s to be missing, got %d: %s", calendar, id, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func deleteViaHTTP(apiURL, calendar, id, etag string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/events/%s?calendar=%s&etag=%s", apiURL, id, calendar, etag), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func mustRequest(t *testing.T, method, url string, body any, headers map[string]string) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func decodeStructured(t *testing.T, raw any, target any) {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}

func containsCalendar(items []events.Calendar, name string) bool {
	for _, item := range items {
		if strings.EqualFold(item.Name, name) || strings.EqualFold(item.DisplayName, name) {
			return true
		}
	}
	return false
}

func containsEvent(items []events.Event, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func containsBusyInterval(items []events.Interval, start, end time.Time) bool {
	for _, item := range items {
		if item.Start.Equal(start) && item.End.Equal(end) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func discoverCalendarCollectionURL(t *testing.T, cfg config.Config, calendar string) (string, bool) {
	t.Helper()

	result := caldavPropfind(
		t,
		cfg,
		discoverCalendarHomeSetURL(t, cfg),
		"1",
		`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:displayname/><d:resourcetype/></d:prop></d:propfind>`,
	)

	key := strings.ToLower(strings.TrimSpace(calendar))
	for _, res := range result.Responses {
		prop, ok := res.okProp()
		if !ok || prop.ResourceType.Calendar == nil {
			continue
		}

		rawName := lastPathSegment(res.Href)
		displayName := strings.TrimSpace(prop.DisplayName)
		if strings.EqualFold(rawName, key) || strings.EqualFold(displayName, key) {
			return absoluteURL(t, cfg, res.Href), true
		}
	}
	return "", false
}

func discoverCalendarHomeSetURL(t *testing.T, cfg config.Config) string {
	t.Helper()

	principalResult := caldavPropfind(
		t,
		cfg,
		strings.TrimRight(cfg.CalDAVBaseURL, "/"),
		"0",
		`<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`,
	)

	principalURL := ""
	for _, res := range principalResult.Responses {
		prop, ok := res.okProp()
		if !ok || prop.CurrentUserPrincipal == nil || strings.TrimSpace(prop.CurrentUserPrincipal.Href) == "" {
			continue
		}
		principalURL = absoluteURL(t, cfg, prop.CurrentUserPrincipal.Href)
		break
	}
	if principalURL == "" {
		t.Fatal("current-user-principal not found")
	}

	homeSetResult := caldavPropfind(
		t,
		cfg,
		principalURL,
		"0",
		`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:calendar-home-set/></d:prop></d:propfind>`,
	)

	for _, res := range homeSetResult.Responses {
		prop, ok := res.okProp()
		if !ok || prop.CalendarHomeSet == nil || strings.TrimSpace(prop.CalendarHomeSet.Href) == "" {
			continue
		}
		return absoluteURL(t, cfg, prop.CalendarHomeSet.Href)
	}

	t.Fatal("calendar-home-set not found")
	return ""
}

func caldavPropfind(t *testing.T, cfg config.Config, rawURL, depth, body string) liveMultiStatus {
	t.Helper()

	req, err := http.NewRequest("PROPFIND", rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new PROPFIND request: %v", err)
	}
	req.SetBasicAuth(cfg.CalDAVUsername, cfg.CalDAVPassword)
	req.Header.Set("Depth", depth)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do PROPFIND request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected PROPFIND status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var result liveMultiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode PROPFIND response: %v", err)
	}
	return result
}

func absoluteURL(t *testing.T, cfg config.Config, raw string) string {
	t.Helper()

	base, err := url.Parse(strings.TrimRight(cfg.CalDAVBaseURL, "/") + "/")
	if err != nil {
		t.Fatalf("parse CalDAV base URL: %v", err)
	}
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("parse CalDAV reference URL: %v", err)
	}
	return base.ResolveReference(ref).String()
}

func joinCollectionURL(t *testing.T, baseURL string, name string) string {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse collection URL: %v", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(strings.TrimSpace(name))
	return u.String()
}

func lastPathSegment(raw string) string {
	u, err := url.Parse(raw)
	if err == nil {
		raw = u.Path
	}
	raw = strings.TrimSuffix(raw, "/")
	parts := strings.Split(raw, "/")
	if len(parts) == 0 {
		return ""
	}
	segment := parts[len(parts)-1]
	decoded, err := url.PathUnescape(segment)
	if err != nil {
		return segment
	}
	return decoded
}

type liveMultiStatus struct {
	Responses []liveResponse `xml:"response"`
}

type liveResponse struct {
	Href     string         `xml:"href"`
	PropStat []livePropStat `xml:"propstat"`
}

type livePropStat struct {
	Status string   `xml:"status"`
	Prop   liveProp `xml:"prop"`
}

type liveProp struct {
	DisplayName          string            `xml:"displayname"`
	ResourceType         liveResourceType  `xml:"resourcetype"`
	CurrentUserPrincipal *liveHrefProperty `xml:"current-user-principal"`
	CalendarHomeSet      *liveHrefProperty `xml:"calendar-home-set"`
}

type liveResourceType struct {
	Collection *struct{} `xml:"collection"`
	Calendar   *struct{} `xml:"calendar"`
}

type liveHrefProperty struct {
	Href string `xml:"href"`
}

func (r liveResponse) okProp() (liveProp, bool) {
	for _, ps := range r.PropStat {
		if strings.Contains(ps.Status, " 200 ") {
			return ps.Prop, true
		}
	}
	return liveProp{}, false
}
