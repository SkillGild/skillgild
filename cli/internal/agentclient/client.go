package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNotSignedIn is returned by signed-in calls when no credential is available.
var ErrNotSignedIn = errors.New("not signed in; run `skillgild login` first")

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// LoadKey, when set, supplies the credential the first time a signed-in request
	// needs one. The MCP server uses it so that a `skillgild login` run after the
	// agent started the server is picked up without restarting the agent.
	LoadKey func() (string, error)

	mu sync.Mutex
}

type Skill struct {
	ID               string          `json:"id"`
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	DistributionMode string          `json:"distribution_mode"`
	AccessTier       string          `json:"access_tier"`
	PriceAmountMinor int64           `json:"price_amount_minor"`
	PriceCurrency    string          `json:"price_currency"`
	IncludedInPro    bool            `json:"included_in_pro"`
	FreeRunsPerMonth int             `json:"free_runs_per_month"`
	CurrentVersion   string          `json:"current_version"`
	InputSchema      json.RawMessage `json:"input_schema"`
	RuntimeType      string          `json:"runtime_type"`
	Tools            []ToolInfo      `json:"tools"`
	// Provenance of a skill packaged from, or (for distribution_mode "open_source")
	// installed from, a public repository. SourcePath is the folder holding SKILL.md.
	SourceURL   string `json:"source_url"`
	SourcePath  string `json:"source_path"`
	License     string `json:"license"`
	Attribution string `json:"attribution"`
}

// DistributionOpenSource marks a community skill: listed by SkillGild, maintained in a
// public repository, installed from source and never run on SkillGild.
const DistributionOpenSource = "open_source"

// ToolInfo is the public interface of a hybrid skill's server tool.
type ToolInfo struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// AgentSession is returned when an agent starts (or resumes) a hybrid skill session.
type AgentSession struct {
	SessionID     string     `json:"session_id"`
	SkillID       string     `json:"skill_id"`
	SkillSlug     string     `json:"skill_slug"`
	SkillName     string     `json:"skill_name"`
	Version       string     `json:"version"`
	ExpiresAt     time.Time  `json:"expires_at"`
	MaxToolCalls  int        `json:"max_tool_calls"`
	ToolCallsUsed int        `json:"tool_calls_used"`
	Resumed       bool       `json:"resumed"`
	Guide         string     `json:"guide"`
	Tools         []ToolInfo `json:"tools"`
}

type ToolCallResult struct {
	SessionID          string          `json:"session_id"`
	Tool               string          `json:"tool"`
	Result             json.RawMessage `json:"result"`
	ToolCallsUsed      int             `json:"tool_calls_used"`
	ToolCallsRemaining int             `json:"tool_calls_remaining"`
	ExpiresAt          time.Time       `json:"expires_at"`
}

type RunResult struct {
	ExecutionID string `json:"execution_id"`
	SkillID     string `json:"skill_id"`
	SkillSlug   string `json:"skill_slug"`
	Version     string `json:"version"`
	Output      string `json:"output"`
	Usage       struct {
		Used       int       `json:"used"`
		Limit      *int      `json:"limit"`
		ResetAt    time.Time `json:"reset_at"`
		AccessTier string    `json:"access_tier"`
	} `json:"usage"`
}

// APIError is an error answer from the SkillGild API.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("SkillGild API request failed (HTTP %d)", e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(baseURL, apiKey string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("API URL must be an absolute HTTP(S) URL")
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, HTTP: &http.Client{Timeout: 100 * time.Second}}, nil
}

// apiKey returns the credential for a signed-in request, loading it on first use.
func (c *Client) apiKey() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.APIKey != "" {
		return c.APIKey, nil
	}
	if c.LoadKey == nil {
		return "", ErrNotSignedIn
	}
	key, err := c.LoadKey()
	if err != nil || strings.TrimSpace(key) == "" {
		return "", ErrNotSignedIn
	}
	c.APIKey = strings.TrimSpace(key)
	return c.APIKey, nil
}

// forgetKey drops a loaded credential the API rejected, so the next call reloads
// whatever `skillgild login` has stored since.
func (c *Client) forgetKey() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.LoadKey != nil {
		c.APIKey = ""
	}
}

func (c *Client) request(ctx context.Context, method, path string, body any, authenticated bool, idempotencyKey string, result any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request")
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		key, err := c.apiKey()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("SkillGild API at %s is unreachable: %w", c.BaseURL, unwrapURLError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read SkillGild response (HTTP %d)", resp.StatusCode)
	}
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &envelope) != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// A proxy or load balancer answered instead of the API; keep the status.
			return &APIError{Status: resp.StatusCode}
		}
		return fmt.Errorf("SkillGild API returned an unexpected response (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode}
		if envelope.Error != nil {
			apiErr.Code, apiErr.Message = envelope.Error.Code, envelope.Error.Message
		}
		if authenticated && resp.StatusCode == http.StatusUnauthorized {
			c.forgetKey()
			if apiErr.Code == "invalid_api_key" {
				apiErr.Message += "; run `skillgild login` to connect again"
			}
		}
		return apiErr
	}
	if result != nil {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("decode SkillGild response")
		}
	}
	return nil
}

// unwrapURLError strips the request method and URL that net/http prefixes, which
// the caller already knows, and keeps the operating system's reason.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}

func (c *Client) Search(ctx context.Context, query string) ([]Skill, error) {
	path := "/skills?limit=100"
	if strings.TrimSpace(query) != "" {
		path += "&q=" + url.QueryEscape(strings.TrimSpace(query))
	}
	var page struct {
		Items []Skill `json:"items"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, false, "", &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (c *Client) GetSkill(ctx context.Context, ref string) (Skill, error) {
	var skill Skill
	err := c.request(ctx, http.MethodGet, "/skills/"+url.PathEscape(ref), nil, false, "", &skill)
	return skill, err
}

func (c *Client) Run(ctx context.Context, ref string, input map[string]any, idempotencyKey ...string) (RunResult, error) {
	var result RunResult
	key := ""
	if len(idempotencyKey) > 0 {
		key = idempotencyKey[0]
	}
	err := c.request(ctx, http.MethodPost, "/skills/"+url.PathEscape(ref)+"/run", map[string]any{"input": input}, true, key, &result)
	return result, err
}

// StartSession starts, or resumes, an agent session for a hybrid skill.
func (c *Client) StartSession(ctx context.Context, ref string) (AgentSession, error) {
	var session AgentSession
	err := c.request(ctx, http.MethodPost, "/skills/"+url.PathEscape(ref)+"/sessions", map[string]any{}, true, "", &session)
	return session, err
}

func (c *Client) CallTool(ctx context.Context, sessionID, tool string, input map[string]any) (ToolCallResult, error) {
	var result ToolCallResult
	err := c.request(ctx, http.MethodPost, "/skill-sessions/"+url.PathEscape(sessionID)+"/tools/"+url.PathEscape(tool), map[string]any{"input": input}, true, "", &result)
	return result, err
}

func (c *Client) EndSession(ctx context.Context, sessionID string) error {
	return c.request(ctx, http.MethodDelete, "/skill-sessions/"+url.PathEscape(sessionID), nil, true, "", nil)
}

// DeviceCode is the start of a device authorization: the code the user approves in
// the browser and the code the CLI polls with.
type DeviceCode struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

func (c *Client) DeviceAuthorization(ctx context.Context, deviceName string) (DeviceCode, error) {
	var result DeviceCode
	err := c.request(ctx, http.MethodPost, "/device-authorizations", map[string]string{"device_name": deviceName, "client_type": "skillgild-cli"}, false, "", &result)
	return result, err
}

// PollDevice reports the authorization state ("authorization_pending" or
// "authorized") and, once authorized, the issued API key.
func (c *Client) PollDevice(ctx context.Context, deviceCode string) (string, string, error) {
	var result struct {
		Status      string `json:"status"`
		AccessToken string `json:"access_token"`
	}
	err := c.request(ctx, http.MethodPost, "/device-authorizations/token", map[string]string{"device_code": deviceCode}, false, "", &result)
	return result.Status, result.AccessToken, err
}

func (c *Client) Me(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodGet, "/me", nil, true, "", &result)
	return result, err
}

// Logout revokes the credential this client authenticates with.
func (c *Client) Logout(ctx context.Context) error {
	return c.request(ctx, http.MethodDelete, "/me/api-keys/current", nil, true, "", nil)
}
