package main

import (
	"context"
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
	if len(args) != 1 {
		return fmt.Errorf("usage: skillgild connect claude-code|codex|cursor|gemini-cli|antigravity|antigravity-cli")
	}
	s, err := newOnboarding()
	if err != nil {
		return err
	}
	if err := s.register(context.Background(), args[0]); err != nil {
		return err
	}
	fmt.Println("Sign in once with `skillgild login` if you have not already.")
	return nil
}

// registerMCP adds or updates the "skillgild" entry under mcpServers in the JSON file
// at path. It reports whether the file changed.
func registerMCP(path, exe string, stdioType bool) (bool, error) {
	return registerMCPWithEnv(path, exe, stdioType, nil)
}

func registerMCPWithEnv(path, exe string, stdioType bool, environment map[string]string) (bool, error) {
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
	if config == nil {
		return false, fmt.Errorf("%s must contain a JSON object, so it was left unchanged", path)
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
		if value, exists := current["env"]; exists {
			if _, ok := value.(map[string]any); !ok {
				return false, fmt.Errorf("skillgild env in %s is not an object, so it was left unchanged", path)
			}
		}
		// Keep anything the user added to the entry, such as env or disabled.
		for k, v := range current {
			if _, set := entry[k]; !set {
				entry[k] = v
			}
		}
		if env, ok := entry["env"].(map[string]any); ok {
			copied := make(map[string]any, len(env))
			for k, v := range env {
				copied[k] = v
			}
			entry["env"] = copied
		}
		mergeMCPEnvironment(entry, environment)
		if reflect.DeepEqual(current, entry) {
			return false, nil
		}
	}
	if current, exists := servers["skillgild"]; exists {
		if _, ok := current.(map[string]any); !ok {
			return false, fmt.Errorf("skillgild in %s is not an object, so it was left unchanged", path)
		}
	}
	mergeMCPEnvironment(entry, environment)
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
	if err := writeConfigAtomically(path, out, mode); err != nil {
		return false, err
	}
	return true, nil
}

func mergeMCPEnvironment(entry map[string]any, environment map[string]string) {
	if len(environment) == 0 {
		return
	}
	env, _ := entry["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	for k, v := range environment {
		env[k] = v
	}
	entry["env"] = env
}

func writeConfigAtomically(path string, out []byte, mode fs.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".skillgild-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(out); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
