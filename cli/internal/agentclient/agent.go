package agentclient

import (
	"context"
	"net/http"
	"os"
	"strings"
)

// AgentHeader names the coding agent driving the CLI, so SkillGild support can see
// which client a user's runs came from.
const AgentHeader = "X-SkillGild-Agent"

type agentKey struct{}

// WithAgent tags requests made with ctx as coming from agent, for example the MCP
// client's "name/version" from its initialize handshake.
func WithAgent(ctx context.Context, agent string) context.Context {
	return context.WithValue(ctx, agentKey{}, agent)
}

// DetectAgent names the coding agent whose terminal the CLI runs in, from the
// environment variables those agents set. It returns "" when none is recognised.
func DetectAgent() string {
	switch {
	case os.Getenv("CLAUDECODE") == "1":
		return "claude-code"
	case os.Getenv("GEMINI_CLI") == "1":
		return "gemini-cli"
	case os.Getenv("CODEX_SANDBOX") != "" || os.Getenv("CODEX_THREAD_ID") != "":
		return "codex"
	case os.Getenv("CURSOR_TRACE_ID") != "" || os.Getenv("CURSOR_AGENT") != "":
		return "cursor"
	}
	return ""
}

func (c *Client) setClientHeaders(req *http.Request) {
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	agent, _ := req.Context().Value(agentKey{}).(string)
	if strings.TrimSpace(agent) == "" {
		agent = DetectAgent()
	}
	if agent != "" {
		req.Header.Set(AgentHeader, agent)
	}
}
