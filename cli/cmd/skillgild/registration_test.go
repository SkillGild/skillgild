package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestNativeRegistration(t *testing.T) {
	for _, agent := range []string{"claude-code", "codex", "gemini-cli"} {
		t.Run(agent, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			environment := map[string]string{"SKILLGILD_API_URL": "http://localhost:8080/v1"}
			const exe = "/Applications/SkillGild Tools/skillgild"
			calls := 0
			run := func(_ context.Context, binary string, args []string) ([]byte, error) {
				calls++
				want := []string{"mcp", "add"}
				if agent != "codex" {
					want = append(want, "--scope", "user")
				}
				want = append(want, "--env", "SKILLGILD_API_URL=http://localhost:8080/v1", "skillgild")
				if agent != "gemini-cli" {
					want = append(want, "--")
				}
				want = append(want, exe, "mcp")
				if !reflect.DeepEqual(args, want) {
					t.Fatalf("args %#v, want %#v", args, want)
				}
				key := "mcpServers"
				if agent == "codex" {
					key = "mcp_servers"
				}
				config := map[string]any{key: map[string]any{"skillgild": map[string]any{"command": exe, "args": []string{"mcp"}, "env": environment}, "other": map[string]any{"command": "other"}}}
				var raw []byte
				var err error
				if agent == "codex" {
					raw, err = toml.Marshal(config)
				} else {
					raw, err = json.Marshal(config)
				}
				if err != nil {
					t.Fatal(err)
				}
				return nil, os.WriteFile(path, raw, 0600)
			}
			for range 2 {
				if err := registerNativeMCP(context.Background(), "agent", agent, path, exe, environment, run); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatalf("repeat setup ran mcp add %d times", calls)
			}
			raw, _ := os.ReadFile(path)
			if !strings.Contains(string(raw), "other") {
				t.Fatal("other server lost")
			}
			if err := registerNativeMCP(context.Background(), "agent", agent, path, "/different/skillgild", environment, run); err == nil {
				t.Fatal("overwrote different server")
			}
			if calls != 1 {
				t.Fatal("ran command despite configuration conflict")
			}
		})
	}
}

func TestRegistrationFailureAndFalseSuccess(t *testing.T) {
	for _, failure := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "config")
		err := registerNativeMCP(context.Background(), "codex", "codex", path, "/bin/skillgild", nil, func(context.Context, string, []string) ([]byte, error) {
			if failure {
				return []byte("bad config"), errors.New("exit 1")
			}
			return nil, nil
		})
		if err == nil {
			t.Fatal("claimed success without a registered server")
		}
	}
}

func TestJSONConfigurationPreservation(t *testing.T) {
	for _, raw := range []string{"null", "[]", `{"mcpServers":null}`, `{"mcpServers":{"skillgild":null}}`, `{"mcpServers":{"skillgild":"wrong"}}`, `{"mcpServers":{"skillgild":{"env":"wrong"}}}`} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			os.WriteFile(path, []byte(raw), 0600)
			if _, err := registerMCP(path, "/bin/skillgild", true); err == nil {
				t.Fatal("rewrote malformed config")
			}
			after, _ := os.ReadFile(path)
			if string(after) != raw {
				t.Fatal("malformed config changed")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{"theme":"dark","mcpServers":{"other":{"command":"node"},"skillgild":{"command":"/old","args":["mcp"],"env":{"KEEP":"1"},"disabled":false}}}`
	os.WriteFile(path, []byte(original), 0600)
	environment := map[string]string{"SKILLGILD_API_URL": "http://localhost:8080/v1"}
	if changed, err := registerMCPWithEnv(path, "/new", true, environment); err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	entry, err := readAgentMCPEntry(path, false)
	if err != nil {
		t.Fatal(err)
	}
	env := entry["env"].(map[string]any)
	if env["KEEP"] != "1" || env["SKILLGILD_API_URL"] != environment["SKILLGILD_API_URL"] {
		t.Fatal(env)
	}
	if changed, err := registerMCPWithEnv(path, "/new", true, environment); err != nil || changed {
		t.Fatalf("repeat: %v %v", changed, err)
	}
	backup, _ := os.ReadFile(path + ".bak")
	if string(backup) != original {
		t.Fatal("original backup not retained")
	}
}

func TestMCPEnvironmentDoesNotPersistCredentials(t *testing.T) {
	t.Setenv("SKILLGILD_API_KEY", "do-not-persist")
	t.Setenv("SKILLGILD_API_URL", "http://localhost:8080/v1")
	t.Setenv("SKILLGILD_CONFIG_DIR", t.TempDir())
	env := mcpEnvironment()
	if env["SKILLGILD_API_KEY"] != "" || env["SKILLGILD_API_URL"] == "" || env["SKILLGILD_CONFIG_DIR"] == "" {
		t.Fatal("incorrect environment propagation")
	}
}
