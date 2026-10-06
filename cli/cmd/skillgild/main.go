package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

const defaultAPIURL = "https://api.skillgild.dev/v1"

// version is set by the release build with -ldflags "-X main.version=v1.2.3".
var version = "dev"

var safeSkillSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	baseURL := os.Getenv("SKILLGILD_API_URL")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	var err error
	switch os.Args[1] {
	case "login":
		err = login(ctx, baseURL)
	case "logout":
		err = logout(ctx, baseURL)
	case "status":
		err = status(ctx, baseURL)
	case "search":
		err = search(ctx, baseURL, strings.Join(os.Args[2:], " "))
	case "install":
		err = install(ctx, baseURL, os.Args[2:])
	case "run":
		err = run(ctx, baseURL, os.Args[2:])
	case "session":
		err = startSession(ctx, baseURL, os.Args[2:])
	case "call":
		err = callTool(ctx, baseURL, os.Args[2:])
	case "end":
		err = endSession(ctx, baseURL, os.Args[2:])
	case "mcp":
		err = serveMCP(ctx, baseURL)
	case "version", "-v", "--version":
		fmt.Printf("skillgild %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "skillgild: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "skillgild:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "SkillGild agent CLI\n\nUsage:\n  skillgild login\n  skillgild status\n  skillgild search [query]\n  skillgild install <skill-slug> [--agent shared|codex|cursor|claude-code|gemini-cli] [--target path] [--force]\n  skillgild run <skill-id-or-slug> --input '{\"prompt\":\"...\"}' [--idempotency-key value]\n  skillgild session <skill-id-or-slug>\n  skillgild call <session-id> <tool> --input '{...}'\n  skillgild end <session-id>\n  skillgild mcp\n  skillgild logout\n  skillgild version\n\nEnvironment:\n  SKILLGILD_API_URL   API endpoint (default "+defaultAPIURL+"; use http://localhost:8080/v1 for a local API)\n  SKILLGILD_API_KEY   use this API key instead of the one saved by `skillgild login`")
}

// newClient returns an API client. Signed-in clients load the credential lazily, so
// a long-running MCP server picks up a login that happens after it started.
func newClient(baseURL string, authenticated bool) (*agentclient.Client, error) {
	api, err := agentclient.New(baseURL, "")
	if err != nil {
		return nil, err
	}
	if authenticated {
		api.LoadKey = agentclient.LoadAPIKey
	}
	return api, nil
}

func login(ctx context.Context, baseURL string) error {
	api, err := newClient(baseURL, false)
	if err != nil {
		return err
	}
	if os.Getenv(agentclient.EnvAPIKey) != "" {
		fmt.Fprintf(os.Stderr, "Note: %s is set and takes precedence over the credential this login saves.\n", agentclient.EnvAPIKey)
	}
	// The credential this login replaces. It is revoked only after the new one is
	// saved, so a login that times out leaves the existing connection working.
	previous, _ := agentclient.LoadStoredAPIKey()

	deviceName, _ := os.Hostname()
	if strings.TrimSpace(deviceName) == "" {
		deviceName = "SkillGild CLI device"
	}
	device, err := api.DeviceAuthorization(ctx, deviceName)
	if err != nil {
		return err
	}
	fmt.Printf("Open %s\nEnter code: %s\n", device.VerificationURI, device.UserCode)
	openBrowser(device.VerificationURI)
	interval := time.Duration(device.IntervalSeconds) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	// The server says when the code expires; a client clock that is far off must not
	// end the wait early, so anything outside the server's window falls back to it.
	wait := time.Until(device.ExpiresAt)
	if wait <= 0 || wait > 15*time.Minute {
		wait = 10 * time.Minute
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("authorization expired; run `skillgild login` again")
		case <-ticker.C:
			state, secret, err := api.PollDevice(ctx, device.DeviceCode)
			var apiErr *agentclient.APIError
			if errors.As(err, &apiErr) && apiErr.Code == "slow_down" {
				continue
			}
			if err != nil {
				return err
			}
			if state != "authorized" {
				continue
			}
			source, err := agentclient.StoreAPIKey(secret)
			if err != nil {
				return fmt.Errorf("authorization succeeded, but the credential could not be saved: %w", err)
			}
			if previous != "" && previous != secret {
				revokePrevious(ctx, baseURL, previous)
			}
			fmt.Printf("Connected. SkillGild saved your revocable credential in %s.\n", source)
			if source == agentclient.SourceFile {
				if path, err := agentclient.CredentialFilePath(); err == nil {
					fmt.Printf("No operating system credential store was available, so it is in %s, readable only by you.\n", path)
				}
			}
			return nil
		}
	}
}

// revokePrevious revokes the credential a new login replaced. Without this, every
// login added a key until the account's five-key limit blocked the next one.
func revokePrevious(ctx context.Context, baseURL, previous string) {
	old, err := agentclient.New(baseURL, previous)
	if err != nil {
		return
	}
	revokeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := old.Logout(revokeCtx); err != nil {
		var apiErr *agentclient.APIError
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			return // already revoked or expired
		}
		fmt.Fprintln(os.Stderr, "Note: the previous credential could not be revoked; disconnect the old device under Account → Agent connections.")
	}
}

func openBrowser(target string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}

func status(ctx context.Context, baseURL string) error {
	key, source, err := agentclient.LoadAPIKeyWithSource()
	if err != nil {
		return fmt.Errorf("not connected to %s; run `skillgild login`", baseURL)
	}
	api, err := agentclient.New(baseURL, key)
	if err != nil {
		return err
	}
	user, err := api.Me(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Connected to SkillGild as %v\nAPI: %s\nCredential: %s\n", user["email"], baseURL, source)
	return nil
}

func logout(ctx context.Context, baseURL string) error {
	key, _, loadErr := agentclient.LoadAPIKeyWithSource()
	var remoteErr error
	if loadErr == nil {
		api, err := agentclient.New(baseURL, key)
		if err != nil {
			return err
		}
		remoteErr = api.Logout(ctx)
		var apiErr *agentclient.APIError
		if errors.As(remoteErr, &apiErr) && apiErr.Status == 401 {
			remoteErr = nil // already revoked or expired
		}
	}
	storeErr := agentclient.DeleteAPIKey()
	switch {
	case storeErr != nil && remoteErr != nil:
		return fmt.Errorf("remote revoke failed (%v) and the local credential could not be removed: %w", remoteErr, storeErr)
	case storeErr != nil:
		return fmt.Errorf("the credential was revoked, but could not be removed locally: %w", storeErr)
	case loadErr != nil:
		fmt.Println("Not connected; nothing to remove.")
		return nil
	case remoteErr != nil:
		return fmt.Errorf("the local credential was removed, but SkillGild could not revoke it (%v); disconnect this device under Account → Agent connections", remoteErr)
	}
	if os.Getenv(agentclient.EnvAPIKey) != "" {
		fmt.Printf("Disconnected. Note: %s is still set in this shell.\n", agentclient.EnvAPIKey)
		return nil
	}
	fmt.Println("Disconnected. The credential was revoked and removed from this machine.")
	return nil
}

func search(ctx context.Context, baseURL, query string) error {
	api, err := newClient(baseURL, false)
	if err != nil {
		return err
	}
	items, err := api.Search(ctx, query)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("No hosted skills found.")
		return nil
	}
	for _, skill := range items {
		access := "free"
		if skill.IncludedInPro {
			access = "pro"
		} else if skill.AccessTier == "paid" {
			access = fmt.Sprintf("%s %.2f", skill.PriceCurrency, float64(skill.PriceAmountMinor)/100)
		}
		fmt.Printf("%-24s %-8s %s — %s\n", skill.Slug, access, skill.Name, skill.Description)
	}
	return nil
}

func install(ctx context.Context, baseURL string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provide a skill slug")
	}
	ref := args[0]
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	agent := flags.String("agent", "shared", "skill directory preset")
	target := flags.String("target", "", "custom skill root directory")
	force := flags.Bool("force", false, "replace an existing installed wrapper")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	api, err := newClient(baseURL, false)
	if err != nil {
		return err
	}
	skill, err := api.GetSkill(ctx, ref)
	if err != nil {
		return err
	}
	if !safeSkillSlug.MatchString(skill.Slug) {
		return fmt.Errorf("the skill has an invalid install slug; no files were written")
	}
	root := *target
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home directory")
		}
		switch *agent {
		case "shared", "codex":
			root = filepath.Join(home, ".agents", "skills")
		case "cursor":
			root = filepath.Join(home, ".cursor", "skills")
		case "claude-code":
			root = filepath.Join(home, ".claude", "skills")
		case "gemini-cli":
			root = filepath.Join(home, ".gemini", "skills")
		default:
			return fmt.Errorf("unsupported agent preset %q; use shared, codex, cursor, claude-code, gemini-cli, or --target", *agent)
		}
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("skill target must be an absolute path")
	}
	skillDir := filepath.Join(root, skill.Slug)
	if _, err := os.Stat(skillDir); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass --force to replace it", skillDir)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill directory %s: %w", skillDir, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(wrapper(skill)), 0o644); err != nil {
		return fmt.Errorf("write agent skill wrapper: %w", err)
	}
	fmt.Printf("Installed hosted wrapper at %s\n", filepath.Join(skillDir, "SKILL.md"))
	fmt.Println("The wrapper contains public metadata only. Connect the SkillGild MCP server (`skillgild mcp`) in your agent to run the skill.")
	return nil
}

func wrapper(skill agentclient.Skill) string {
	if skill.RuntimeType == "hybrid_tools" {
		return hybridWrapper(skill)
	}
	access := "Free hosted skill"
	if skill.AccessTier == "paid" {
		access = fmt.Sprintf("Paid hosted skill (%s %.2f)", skill.PriceCurrency, float64(skill.PriceAmountMinor)/100)
	}
	var schema bytes.Buffer
	if len(skill.InputSchema) == 0 || json.Indent(&schema, skill.InputSchema, "", "  ") != nil {
		schema.WriteString(`{"type":"object"}`)
	}
	description := strings.Join(strings.Fields(skill.Description), " ")
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\n%s. SkillGild keeps the private implementation on its hosted service. This local file is only a reference and invocation guide.\n\nWhen the user's coding task matches this skill, use the SkillGild MCP tool `skillgild_run_skill` with skill ID `%s` and an input object conforming to this public input schema:\n\n```json\n%s\n```\n\nDo not ask the user to paste credentials into chat. The SkillGild MCP process authenticates through the local operating system credential store. Explain that request content is sent to SkillGild's configured AI provider before running if the user has not already agreed to that for this task. Use the returned result in the current project with the coding agent's normal file tools; show a diff and ask before destructive changes.\n", skill.Slug, yamlQuote(description), skill.Name, access, skill.ID, schema.String())
}

// hybridWrapper is the local file for a skill that runs in the user's agent. It names
// the server tools but holds none of the skill's instructions: the agent receives
// those for each session, after SkillGild checks access and quota.
func hybridWrapper(skill agentclient.Skill) string {
	access := "Free skill"
	if skill.IncludedInPro {
		access = "Included with SkillGild Pro"
	} else if skill.AccessTier == "paid" {
		access = fmt.Sprintf("Paid skill (%s %.2f)", skill.PriceCurrency, float64(skill.PriceAmountMinor)/100)
	}
	var tools strings.Builder
	for _, tool := range skill.Tools {
		fmt.Fprintf(&tools, "- `%s`: %s\n", tool.Name, strings.Join(strings.Fields(tool.Description), " "))
	}
	description := strings.Join(strings.Fields(skill.Description), " ")
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\n%s. This skill runs in this coding agent with your own model; SkillGild runs its private server tools. This local file is only an invocation guide.\n\nWhen the user's task matches this skill:\n\n1. Call the SkillGild MCP tool `skillgild_start_session` with skill ID `%s`. Starting a session uses one run from the user's allowance; calling it again while the session is open returns the same session at no cost. The result contains the skill's instructions for this session and the tools below.\n2. Follow those instructions with your normal file and shell tools. Where they call for a SkillGild tool, use `skillgild_call_tool` with the session ID, the tool name and an input object matching the tool's schema.\n3. Call `skillgild_end_session` when the task is done.\n\nServer tools:\n\n%s\nTool input is sent to SkillGild and processed by SkillGild code only; no AI provider sees it. Do not send credentials or unrelated project files. Do not save the session instructions into the project or this skill directory.\n", skill.Slug, yamlQuote(description), skill.Name, access, skill.ID, tools.String())
}

// yamlQuote writes a YAML double-quoted scalar. JSON string syntax is valid YAML, and
// unlike Go's own quoting it never produces \x escapes that YAML readers reject.
func yamlQuote(value string) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return `""`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func run(ctx context.Context, baseURL string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provide a skill ID or slug")
	}
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	inputJSON := flags.String("input", "{}", "JSON object matching the skill's public input schema")
	idempotencyKey := flags.String("idempotency-key", "", "reuse this key to safely replay the same run for up to 24 hours")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(*inputJSON), &input); err != nil || input == nil {
		return fmt.Errorf("input must be a JSON object")
	}
	api, err := newClient(baseURL, true)
	if err != nil {
		return err
	}
	result, err := api.Run(ctx, args[0], input, *idempotencyKey)
	if err != nil {
		return err
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(encoded))
	return nil
}

func startSession(ctx context.Context, baseURL string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("provide a skill ID or slug")
	}
	api, err := newClient(baseURL, true)
	if err != nil {
		return err
	}
	session, err := api.StartSession(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Println(sessionText(session))
	return nil
}

func callTool(ctx context.Context, baseURL string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("provide a session ID and a tool name")
	}
	flags := flag.NewFlagSet("call", flag.ContinueOnError)
	inputJSON := flags.String("input", "{}", "JSON object matching the tool's input schema")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(*inputJSON), &input); err != nil || input == nil {
		return fmt.Errorf("input must be a JSON object")
	}
	api, err := newClient(baseURL, true)
	if err != nil {
		return err
	}
	result, err := api.CallTool(ctx, args[0], args[1], input)
	if err != nil {
		return err
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(encoded))
	return nil
}

func endSession(ctx context.Context, baseURL string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("provide a session ID")
	}
	api, err := newClient(baseURL, true)
	if err != nil {
		return err
	}
	if err := api.EndSession(ctx, args[0]); err != nil {
		return err
	}
	fmt.Println("Session ended.")
	return nil
}

// sessionText renders a session for an agent to read: what it may call and for how
// long, then the skill's instructions as Markdown rather than an escaped JSON string.
func sessionText(session agentclient.AgentSession) string {
	var b strings.Builder
	state := "Started"
	if session.Resumed {
		state = "Resumed"
	}
	fmt.Fprintf(&b, "# SkillGild session: %s v%s\n\n%s session `%s`. It expires at %s and has %d of %d tool calls left.\n\n", session.SkillName, session.Version, state, session.SessionID, session.ExpiresAt.Format(time.RFC3339), session.MaxToolCalls-session.ToolCallsUsed, session.MaxToolCalls)
	b.WriteString("Call these with `skillgild_call_tool` (or `skillgild call <session-id> <tool> --input '{...}'`):\n")
	for _, tool := range session.Tools {
		schema, _ := json.Marshal(tool.InputSchema)
		var compact bytes.Buffer
		if json.Compact(&compact, schema) != nil {
			compact.Reset()
			compact.Write(schema)
		}
		fmt.Fprintf(&b, "\n## `%s`: %s\n\n%s\n\nInput schema: `%s`\n", tool.Name, tool.Title, tool.Description, compact.String())
	}
	b.WriteString("\n---\n\n")
	b.WriteString(session.Guide)
	return b.String()
}

// serveMCP runs the stdio MCP server. It starts without a credential: search works,
// and every signed-in tool tells the agent to run `skillgild login`. The credential
// is read when a tool first needs it, so a login that happens after the agent
// launched this server is picked up without a restart.
func serveMCP(ctx context.Context, baseURL string) error {
	api, err := newClient(baseURL, true)
	if err != nil {
		return err
	}
	return newMCPServer(api).Run(ctx, &mcp.StdioTransport{})
}

func newMCPServer(api *agentclient.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "skillgild", Version: version}, &mcp.ServerOptions{
		Instructions: "Discover SkillGild skills with skillgild_search_skills. Each skill has a runtime_type. prompt_pipeline skills run on SkillGild: call skillgild_run_skill, which sends the input to SkillGild and its configured AI provider. hybrid_tools skills run here, in this agent: call skillgild_start_session, follow the instructions it returns, call the listed server tools with skillgild_call_tool, and finish with skillgild_end_session. Server tools run SkillGild code only, with no AI provider. If a tool answers that the user is not signed in, ask the user to run `skillgild login` in a terminal, then call the tool again. Never pass credentials, secret values, or unrelated project files unless the user approved sharing them.",
	})
	mcp.AddTool(server, &mcp.Tool{Name: "skillgild_search_skills", Description: "Search SkillGild's public hosted skill catalog. Returns public metadata, access tier, version, and public input schema; never returns private prompts or implementation."}, func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
		Query string `json:"query" jsonschema:"optional search phrase for hosted skills"`
	}) (*mcp.CallToolResult, any, error) {
		items, err := api.Search(ctx, args.Query)
		if err != nil {
			return nil, nil, err
		}
		return jsonToolResult(items), items, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "skillgild_run_skill", Description: "Run a SkillGild prompt_pipeline skill by public ID/slug and JSON input. For hybrid_tools skills use skillgild_start_session instead. The input is sent to SkillGild's configured AI provider, may consume a free allowance or paid entitlement, and generated output is returned. Never send credentials or unrelated sensitive project data."}, func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
		SkillID string         `json:"skill_id" jsonschema:"public SkillGild skill ID or slug"`
		Input   map[string]any `json:"input" jsonschema:"input object matching the skill's public input schema"`
	}) (*mcp.CallToolResult, any, error) {
		if args.SkillID == "" || args.Input == nil {
			return nil, nil, errors.New("skill_id and input object are required")
		}
		result, err := api.Run(ctx, args.SkillID, args.Input)
		if err != nil {
			return nil, nil, err
		}
		return jsonToolResult(result), result, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "skillgild_start_session", Description: "Start a session for a SkillGild hybrid_tools skill. Uses one run from the user's free allowance or paid entitlement; while a session is open, calling this again returns it at no cost. Returns the skill's instructions for this agent to follow and the server tools it may call, with the session's expiry and tool call limit."}, func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
		SkillID string `json:"skill_id" jsonschema:"public SkillGild skill ID or slug"`
	}) (*mcp.CallToolResult, any, error) {
		if args.SkillID == "" {
			return nil, nil, errors.New("skill_id is required")
		}
		session, err := api.StartSession(ctx, args.SkillID)
		if err != nil {
			return nil, nil, err
		}
		// Clients that read structuredContent (Claude Code does) never see the text
		// content, so the skill's instructions must be in the structured result too.
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: sessionText(session)}}}, map[string]any{"session_id": session.SessionID, "skill_name": session.SkillName, "version": session.Version, "expires_at": session.ExpiresAt, "max_tool_calls": session.MaxToolCalls, "tool_calls_used": session.ToolCallsUsed, "resumed": session.Resumed, "tools": session.Tools, "instructions": session.Guide}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "skillgild_call_tool", Description: "Call a server tool in an open SkillGild session. The input is processed by SkillGild code only, never by an AI provider, and is not stored. Each call, including one with invalid input, uses one of the session's tool calls."}, func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
		SessionID string         `json:"session_id" jsonschema:"session ID from skillgild_start_session"`
		Tool      string         `json:"tool" jsonschema:"tool name listed by the session"`
		Input     map[string]any `json:"input" jsonschema:"input object matching the tool's input schema"`
	}) (*mcp.CallToolResult, any, error) {
		if args.SessionID == "" || args.Tool == "" || args.Input == nil {
			return nil, nil, errors.New("session_id, tool and input object are required")
		}
		result, err := api.CallTool(ctx, args.SessionID, args.Tool, args.Input)
		if err != nil {
			return nil, nil, err
		}
		return jsonToolResult(result), result, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "skillgild_end_session", Description: "End a SkillGild session when the task is done, so its remaining tool calls can no longer be used."}, func(ctx context.Context, _ *mcp.CallToolRequest, args struct {
		SessionID string `json:"session_id" jsonschema:"session ID from skillgild_start_session"`
	}) (*mcp.CallToolResult, any, error) {
		if args.SessionID == "" {
			return nil, nil, errors.New("session_id is required")
		}
		if err := api.EndSession(ctx, args.SessionID); err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Session ended."}}}, map[string]any{"ended": true}, nil
	})
	return server
}

func jsonToolResult(value any) *mcp.CallToolResult {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Could not encode SkillGild response."}}}
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}
}
