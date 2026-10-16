package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRegisterMCP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "mcp_config.json")

	// A missing file is created with just SkillGild.
	if changed, err := registerMCP(path, "/bin/skillgild", false); err != nil || !changed {
		t.Fatalf("create: %v %v", changed, err)
	}
	entry := readConfig(t, path)["mcpServers"].(map[string]any)["skillgild"].(map[string]any)
	if entry["command"] != "/bin/skillgild" || entry["args"].([]any)[0] != "mcp" || entry["type"] != nil {
		t.Fatalf("entry: %v", entry)
	}
	// Running it again changes nothing.
	if changed, err := registerMCP(path, "/bin/skillgild", false); err != nil || changed {
		t.Fatalf("repeat: %v %v", changed, err)
	}

	// Other servers, other top-level keys and the user's own fields are kept; a backup is written.
	os.Remove(path)
	os.WriteFile(path, []byte(`{"theme":"dark","mcpServers":{"other":{"command":"node"},"skillgild":{"command":"/old","args":["mcp"],"env":{"A":"1"}}}}`), 0o600)
	if changed, err := registerMCP(path, "/new/skillgild", true); err != nil || !changed {
		t.Fatalf("update: %v %v", changed, err)
	}
	c := readConfig(t, path)
	servers := c["mcpServers"].(map[string]any)
	got := servers["skillgild"].(map[string]any)
	if c["theme"] != "dark" || servers["other"] == nil || got["command"] != "/new/skillgild" || got["type"] != "stdio" || got["env"] == nil {
		t.Fatalf("merged: %v", c)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", info.Mode())
	}
	if b, err := os.ReadFile(path + ".bak"); err != nil || len(b) == 0 {
		t.Fatalf("backup: %v", err)
	}

	// An invalid file is left alone.
	os.WriteFile(path, []byte(`{not json`), 0o644)
	if _, err := registerMCP(path, "/bin/skillgild", false); err == nil {
		t.Fatal("rewrote invalid JSON")
	}
	if b, _ := os.ReadFile(path); string(b) != `{not json` {
		t.Fatal("invalid file changed")
	}
}
