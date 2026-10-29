package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type agentCommand func(context.Context, string, []string) ([]byte, error)

func runAgentCommand(ctx context.Context, binary string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, binary, args...).CombinedOutput()
}

// Only non-secret runtime settings are copied. Credentials stay in the existing
// credential store and are loaded by the MCP process when a tool is called.
func mcpEnvironment() map[string]string {
	env := map[string]string{}
	for _, key := range []string{"SKILLGILD_API_URL", "SKILLGILD_CONFIG_DIR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env[key] = value
		}
	}
	return env
}

func connectAgent(ctx context.Context, home, exe, agent string, lookup func(string) (string, error), run agentCommand) error {
	environment := mcpEnvironment()
	if target, ok := connectTargets[agent]; ok {
		path := filepath.Join(home, target.path)
		entry, err := readAgentMCPEntry(path, false)
		if err != nil {
			return err
		}
		if entry != nil && (entry["disabled"] == true || entry["enabled"] == false) {
			return fmt.Errorf("SkillGild is disabled in %s; enable it in your agent before running setup again", path)
		}
		changed, err := registerMCPWithEnv(path, exe, target.stdioType, environment)
		if err != nil {
			return err
		}
		if changed {
			fmt.Printf("Connected SkillGild to %s.\n", target.name)
		} else {
			fmt.Printf("SkillGild is already connected to %s.\n", target.name)
		}
		fmt.Println(target.after)
		return nil
	}
	var binaryName, path string
	switch agent {
	case "claude-code":
		binaryName, path = "claude", filepath.Join(home, ".claude.json")
		if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			path = filepath.Join(dir, ".claude.json")
		}
	case "codex":
		binaryName, path = "codex", filepath.Join(home, ".codex", "config.toml")
		if dir := os.Getenv("CODEX_HOME"); dir != "" {
			path = filepath.Join(dir, "config.toml")
		}
	case "gemini-cli":
		binaryName, path = "gemini", filepath.Join(home, ".gemini", "settings.json")
		if dir := os.Getenv("GEMINI_CLI_HOME"); dir != "" {
			path = filepath.Join(dir, ".gemini", "settings.json")
		}
	default:
		return fmt.Errorf("unsupported agent %q; use claude-code, codex, cursor, gemini-cli, antigravity or antigravity-cli: %w", agent, errNeedsNewer)
	}
	binary, err := lookup(binaryName)
	if err != nil {
		return fmt.Errorf("%s CLI was not found on PATH; install it before connecting SkillGild", binaryName)
	}
	return registerNativeMCP(ctx, binary, agent, path, exe, environment, run)
}

func registerNativeMCP(ctx context.Context, binary, agent, path, exe string, environment map[string]string, run agentCommand) error {
	entry, err := readAgentMCPEntry(path, agent == "codex")
	if err != nil {
		return err
	}
	if entry != nil {
		if matchingMCPEntry(entry, exe, environment) {
			fmt.Printf("SkillGild is already connected to %s.\n", agent)
			return nil
		}
		return fmt.Errorf("%s already has a different skillgild MCP configuration in %s; it was left unchanged. Review or remove that entry, then rerun skillgild setup --agent %s", agent, path, agent)
	}
	args := []string{"mcp", "add"}
	if agent != "codex" {
		args = append(args, "--scope", "user")
	}
	var keys []string
	for k := range environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+environment[key])
	}
	args = append(args, "skillgild")
	if agent != "gemini-cli" {
		args = append(args, "--")
	}
	args = append(args, exe, "mcp")
	output, err := run(ctx, binary, args)
	if err != nil {
		return fmt.Errorf("%s MCP registration failed: %w\n%s", agent, err, strings.TrimSpace(string(output)))
	}
	entry, err = readAgentMCPEntry(path, agent == "codex")
	if err != nil {
		return err
	}
	if !matchingMCPEntry(entry, exe, environment) {
		return fmt.Errorf("%s returned success, but its SkillGild MCP entry could not be verified in %s", agent, path)
	}
	fmt.Printf("Connected SkillGild to %s.\n", agent)
	return nil
}

func readAgentMCPEntry(path string, isTOML bool) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if isTOML {
		err = toml.Unmarshal(raw, &config)
	} else {
		err = json.Unmarshal(raw, &config)
	}
	if err != nil || config == nil {
		return nil, fmt.Errorf("could not read %s as an agent configuration; it was left unchanged", path)
	}
	key := "mcpServers"
	if isTOML {
		key = "mcp_servers"
	}
	servers, ok := config[key].(map[string]any)
	if !ok {
		if _, exists := config[key]; exists {
			return nil, fmt.Errorf("%s in %s is not an object; configuration was left unchanged", key, path)
		}
		return nil, nil
	}
	entry, ok := servers["skillgild"].(map[string]any)
	if !ok {
		if _, exists := servers["skillgild"]; exists {
			return nil, fmt.Errorf("skillgild in %s is not an object; configuration was left unchanged", path)
		}
		return nil, nil
	}
	return entry, nil
}

func matchingMCPEntry(entry map[string]any, exe string, environment map[string]string) bool {
	command, ok := entry["command"].(string)
	if !ok || !sameExecutable(command, exe) {
		return false
	}
	// TOML and JSON both decode these values into []any.
	if !reflect.DeepEqual(entry["args"], []any{"mcp"}) {
		return false
	}
	env, _ := entry["env"].(map[string]any)
	for k, v := range environment {
		if env[k] != v {
			return false
		}
	}
	if enabled, ok := entry["enabled"].(bool); ok && !enabled {
		return false
	}
	if disabled, ok := entry["disabled"].(bool); ok && disabled {
		return false
	}
	return true
}

func sameExecutable(command, exe string) bool {
	if command == exe {
		return true
	}
	if !filepath.IsAbs(command) {
		if path, err := exec.LookPath(command); err == nil {
			command = path
		}
	}
	if path, err := filepath.EvalSymlinks(command); err == nil {
		command = path
	}
	if path, err := filepath.EvalSymlinks(exe); err == nil {
		exe = path
	}
	return command == exe
}
