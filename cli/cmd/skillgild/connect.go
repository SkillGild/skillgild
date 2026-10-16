package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

// connectTarget is an agent whose MCP servers live in a JSON file rather than behind
// an "mcp add" command, so `skillgild connect` edits that file for the user.
type connectTarget struct {
	name string
	// path is the global config file, relative to the home directory.
	path string
	// stdioType adds "type": "stdio", which Cursor accepts and Antigravity does not.
	stdioType bool
	after     string
}

var connectTargets = map[string]connectTarget{
	"antigravity": {
		name: "Antigravity",
		path: filepath.Join(".gemini", "config", "mcp_config.json"),
		after: "Reload Antigravity (agent panel, ... menu, MCP Servers) so it starts SkillGild.\n" +
			"Antigravity asks before each tool call; to skip that, allow mcp(skillgild/*) in its permissions.",
	},
	"cursor": {
		name:      "Cursor",
		path:      filepath.Join(".cursor", "mcp.json"),
		stdioType: true,
		after:     "Reload Cursor, then check Settings > MCP shows skillgild as connected.",
	},
}

func init() {
	// The Antigravity CLI reads the same global MCP config as the IDE.
	connectTargets["antigravity-cli"] = connectTargets["antigravity"]
}

// connect registers `skillgild mcp` in an agent's MCP config file. It keeps every other
// server, leaves an unreadable file untouched, and backs the file up before changing it.
func connect(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("name the agent: skillgild connect antigravity (or cursor)")
	}
	target, ok := connectTargets[args[0]]
	if !ok {
		return fmt.Errorf("skillgild connect supports antigravity, antigravity-cli and cursor; for Claude Code, Codex and Gemini CLI use their own mcp add command (see https://skillgild.dev/docs/quickstart-agents)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find the skillgild executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	path := filepath.Join(home, target.path)
	changed, err := registerMCP(path, exe, target.stdioType)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Printf("SkillGild is already connected in %s.\n", path)
	} else {
		fmt.Printf("Connected SkillGild to %s in %s.\n", target.name, path)
	}
	fmt.Println(target.after)
	fmt.Println("Sign in once with `skillgild login` if you have not already.")
	return nil
}

// registerMCP adds or updates the "skillgild" entry under mcpServers in the JSON file
// at path. It reports whether the file changed.
func registerMCP(path, exe string, stdioType bool) (bool, error) {
	config := map[string]any{}
	original, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		original = nil
	case err != nil:
		return false, fmt.Errorf("read %s: %w", path, err)
	case len(original) > 0:
		if err := json.Unmarshal(original, &config); err != nil {
			return false, fmt.Errorf("%s is not valid JSON, so it was left unchanged: %w", path, err)
		}
	}
	servers, _ := config["mcpServers"].(map[string]any)
	if servers == nil {
		if _, exists := config["mcpServers"]; exists {
			return false, fmt.Errorf("mcpServers in %s is not an object, so it was left unchanged", path)
		}
		servers = map[string]any{}
	}
	entry := map[string]any{"command": exe, "args": []any{"mcp"}}
	if stdioType {
		entry["type"] = "stdio"
	}
	if current, ok := servers["skillgild"].(map[string]any); ok {
		// Keep anything the user added to the entry, such as env or disabled.
		for k, v := range current {
			if _, set := entry[k]; !set {
				entry[k] = v
			}
		}
		if reflect.DeepEqual(current, entry) {
			return false, nil
		}
	}
	servers["skillgild"] = entry
	config["mcpServers"] = servers
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode config: %w", err)
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	mode := fs.FileMode(0o644)
	if original != nil {
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(path+".bak", original, mode); err != nil {
			return false, fmt.Errorf("back up %s: %w", path, err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, mode); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}
