package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

// fakeAPI serves the session endpoints the MCP tools call and records each request.
func fakeAPI(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer sg_live_test" {
			t.Errorf("%s %s sent no API key", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/skills/livecanvas/sessions":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"session_id":"11111111-2222-3333-4444-555555555555","skill_name":"LiveCanvas","version":"1.1.0","expires_at":"2026-10-03T12:00:00Z","max_tool_calls":100,"tool_calls_used":0,"guide":"# Guide body","tools":[{"name":"validate_series","title":"Validate","description":"Checks data.json","input_schema":{"type":"object"}}]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/skill-sessions/11111111-2222-3333-4444-555555555555/tools/validate_series":
			var body struct {
				Input map[string]any `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Input["data"] == nil {
				t.Errorf("tool call body did not carry the input object: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"session_id":"11111111-2222-3333-4444-555555555555","tool":"validate_series","result":{"ok":true},"tool_calls_used":1,"tool_calls_remaining":99,"expires_at":"2026-10-03T12:00:00Z"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/skill-sessions/11111111-2222-3333-4444-555555555555":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"unexpected request"}}`))
		}
	}))
}

func TestMCPSessionTools(t *testing.T) {
	var calls []string
	server := fakeAPI(t, &calls)
	defer server.Close()
	api, err := agentclient.New(server.URL+"/v1", "sg_live_test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := newMCPServer(api).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	call := func(name string, args map[string]any) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned an error: %+v", name, result.Content)
		}
		return result.Content[0].(*mcp.TextContent).Text
	}

	started := call("skillgild_start_session", map[string]any{"skill_id": "livecanvas"})
	for _, want := range []string{"Started session `11111111-2222-3333-4444-555555555555`", "100 of 100 tool calls left", "## `validate_series`", "# Guide body"} {
		if !strings.Contains(started, want) {
			t.Errorf("session text is missing %q:\n%s", want, started)
		}
	}
	// Clients that show only structuredContent must still receive the instructions.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "skillgild_start_session", Arguments: map[string]any{"skill_id": "livecanvas"}})
	if err != nil {
		t.Fatal(err)
	}
	structured, _ := json.Marshal(result.StructuredContent)
	if !strings.Contains(string(structured), `"instructions":"# Guide body"`) {
		t.Errorf("structured session result is missing the instructions: %s", structured)
	}
	calls = calls[:1] // the API calls below are checked after the first start
	called := call("skillgild_call_tool", map[string]any{"session_id": "11111111-2222-3333-4444-555555555555", "tool": "validate_series", "input": map[string]any{"data": map[string]any{"schemaVersion": 1}}})
	if !strings.Contains(called, `"tool_calls_remaining": 99`) {
		t.Errorf("tool result missing call count:\n%s", called)
	}
	if ended := call("skillgild_end_session", map[string]any{"session_id": "11111111-2222-3333-4444-555555555555"}); ended != "Session ended." {
		t.Errorf("unexpected end result %q", ended)
	}
	want := []string{
		"POST /v1/skills/livecanvas/sessions",
		"POST /v1/skill-sessions/11111111-2222-3333-4444-555555555555/tools/validate_series",
		"DELETE /v1/skill-sessions/11111111-2222-3333-4444-555555555555",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("API calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestMCPWithoutCredentialPointsToLogin(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	api, err := agentclient.New(server.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := newMCPServer(api).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "skillgild_start_session", Arguments: map[string]any{"skill_id": "livecanvas"}})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !result.IsError || !strings.Contains(text, "skillgild login") {
		t.Fatalf("want an error that points to `skillgild login`, got %v %q", result.IsError, text)
	}
	if len(calls) != 0 {
		t.Fatalf("a signed-in tool reached the API without a credential: %v", calls)
	}
}

func TestHybridWrapperHoldsNoInstructions(t *testing.T) {
	skill := agentclient.Skill{ID: "skill-id", Slug: "livecanvas", Name: "LiveCanvas", Description: "Cards", AccessTier: "free", RuntimeType: "hybrid_tools",
		Tools: []agentclient.ToolInfo{{Name: "validate_series", Description: "Checks data.json"}}}
	text := wrapper(skill)
	for _, want := range []string{"name: livecanvas", "`skillgild_start_session` with skill ID `skill-id`", "- `validate_series`: Checks data.json"} {
		if !strings.Contains(text, want) {
			t.Errorf("wrapper is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "skillgild_run_skill") {
		t.Error("a hybrid wrapper must not point the agent at the hosted run tool")
	}
}
