package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"calendar-api/internal/events"
	"calendar-api/internal/service"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerRegistersToolsAndHandlesCreateEvent(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stub := &stubService{}
	server := New(stub, logger)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 15 {
		t.Fatalf("expected 15 tools, got %d", len(tools.Tools))
	}
	if !hasTool(tools.Tools, "create_event") {
		t.Fatalf("expected create_event tool in %#v", tools.Tools)
	}
	if !hasTool(tools.Tools, "create_todo") {
		t.Fatalf("expected create_todo tool in %#v", tools.Tools)
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_event",
		Arguments: map[string]any{
			"title":    "Checkup",
			"start":    "2026-03-24T12:30:00+01:00",
			"end":      "2026-03-24T13:00:00+01:00",
			"timezone": "Europe/Paris",
			"dryRun":   true,
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful result, got error content %#v", result.Content)
	}

	var structured service.EventResult
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, &structured); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	if !structured.DryRun {
		t.Fatal("expected dryRun result")
	}
	if structured.Event.Title != "Checkup" {
		t.Fatalf("unexpected title: %#v", structured.Event)
	}
	if !stub.created {
		t.Fatal("expected stub CreateEvent to be called")
	}
	if !stub.lastCreate.DryRun {
		t.Fatalf("expected dryRun request, got %#v", stub.lastCreate)
	}

	todoResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_todo",
		Arguments: map[string]any{
			"title":    "File taxes",
			"due":      "2026-04-15T00:00:00+02:00",
			"timezone": "Europe/Paris",
			"dryRun":   true,
		},
	})
	if err != nil {
		t.Fatalf("call create_todo tool: %v", err)
	}
	if todoResult.IsError {
		t.Fatalf("expected successful todo result, got error content %#v", todoResult.Content)
	}

	var structuredTodo service.TodoResult
	rawTodo, err := json.Marshal(todoResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal todo structured content: %v", err)
	}
	if err := json.Unmarshal(rawTodo, &structuredTodo); err != nil {
		t.Fatalf("unmarshal todo structured content: %v", err)
	}
	if !structuredTodo.DryRun {
		t.Fatal("expected dryRun todo result")
	}
	if structuredTodo.Todo.Title != "File taxes" {
		t.Fatalf("unexpected todo title: %#v", structuredTodo.Todo)
	}
	if !stub.todoCreated {
		t.Fatal("expected stub CreateTodo to be called")
	}
}

type stubService struct {
	created     bool
	todoCreated bool
	lastCreate  events.CreateRequest
	lastTodo    events.CreateTodoRequest
}

func (s *stubService) Health(context.Context) (service.HealthResult, error) {
	return service.HealthResult{
		Status: "ok",
		CalDAV: service.HealthUpstream{
			Reachable: true,
		},
	}, nil
}

func (s *stubService) ListCalendars(context.Context) ([]events.Calendar, error) {
	return []events.Calendar{{Name: "wall", DisplayName: "Wall", Href: "/vince/wall/", Source: events.SourceCalDAV}}, nil
}

func (s *stubService) ListEvents(context.Context, service.ListEventsParams) ([]events.Event, error) {
	return nil, nil
}

func (s *stubService) UpcomingEvents(context.Context, service.UpcomingEventsParams) ([]events.Event, error) {
	return nil, nil
}

func (s *stubService) GetEvent(context.Context, service.GetEventParams) (events.Event, error) {
	return events.Event{}, errors.New("event not found")
}

func (s *stubService) CreateEvent(_ context.Context, req events.CreateRequest) (service.EventResult, error) {
	s.created = true
	s.lastCreate = req
	start, _ := time.Parse(time.RFC3339, req.Start)
	end, _ := time.Parse(time.RFC3339, req.End)
	return service.EventResult{
		DryRun: req.DryRun,
		Event: events.Event{
			ID:       "checkup-1234",
			Calendar: "wall",
			Title:    req.Title,
			Start:    start,
			End:      end,
			Timezone: req.Timezone,
			Source:   events.SourceCalDAV,
		},
	}, nil
}

func (s *stubService) ListTodos(context.Context, service.ListTodosParams) ([]events.Todo, error) {
	return nil, nil
}

func (s *stubService) GetTodo(context.Context, service.GetTodoParams) (events.Todo, error) {
	return events.Todo{}, errors.New("todo not found")
}

func (s *stubService) CreateTodo(_ context.Context, req events.CreateTodoRequest) (service.TodoResult, error) {
	s.todoCreated = true
	s.lastTodo = req

	var due *time.Time
	if req.Due != "" {
		parsedDue, _ := time.Parse(time.RFC3339, req.Due)
		due = &parsedDue
	}
	return service.TodoResult{
		DryRun: req.DryRun,
		Todo: events.Todo{
			ID:              "file-taxes-1234",
			Calendar:        "wall",
			Title:           req.Title,
			Due:             due,
			Timezone:        req.Timezone,
			Status:          events.TodoStatusNeedsAction,
			PercentComplete: 0,
			Source:          events.SourceCalDAV,
		},
	}, nil
}

func (s *stubService) PatchEventWithETag(context.Context, service.PatchEventParams, string) (service.EventResult, error) {
	return service.EventResult{}, errors.New("not implemented")
}

func (s *stubService) PatchTodoWithETag(context.Context, service.PatchTodoParams, string) (service.TodoResult, error) {
	return service.TodoResult{}, errors.New("not implemented")
}

func (s *stubService) MoveEventWithETag(context.Context, service.MoveEventParams, string) (service.EventResult, error) {
	return service.EventResult{}, errors.New("not implemented")
}

func (s *stubService) DeleteEvent(context.Context, service.DeleteEventParams) (service.DeleteResult, error) {
	return service.DeleteResult{}, errors.New("not implemented")
}

func (s *stubService) DeleteTodo(context.Context, service.DeleteTodoParams) (service.DeleteResult, error) {
	return service.DeleteResult{}, errors.New("not implemented")
}

func (s *stubService) Availability(context.Context, service.AvailabilityParams) (service.AvailabilityResult, error) {
	return service.AvailabilityResult{}, errors.New("not implemented")
}

func hasTool(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
