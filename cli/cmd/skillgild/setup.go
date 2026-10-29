package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

type setupAgent struct{ id, name, binary, directory string }

var setupAgents = []setupAgent{
	{"claude-code", "Claude Code", "claude", ".claude"},
	{"codex", "Codex", "codex", ".codex"},
	{"cursor", "Cursor", "cursor", ".cursor"},
	{"gemini-cli", "Gemini CLI", "gemini", ""},
	{"antigravity", "Antigravity", "antigravity", filepath.Join(".gemini", "config")},
	{"antigravity-cli", "Antigravity CLI", "agy", filepath.Join(".gemini", "antigravity-cli")},
}

// Dependencies are scoped to one invocation so tests never touch a real account,
// keyring, browser or agent configuration.
type onboarding struct {
	home, exe, preferencePath string
	in                        io.Reader
	out                       io.Writer
	interactive               bool
	lookup                    func(string) (string, error)
	authenticate              func(context.Context, string, bool) error
	register                  func(context.Context, string) error
}

func newOnboarding() (*onboarding, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	credentialPath, err := agentclient.CredentialFilePath()
	if err != nil {
		return nil, err
	}
	s := &onboarding{home: home, exe: exe, preferencePath: filepath.Join(filepath.Dir(credentialPath), "setup.json"), in: os.Stdin, out: os.Stdout,
		interactive: isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd()), lookup: exec.LookPath, authenticate: ensureSetupLogin}
	s.register = func(ctx context.Context, agent string) error {
		return connectAgent(ctx, s.home, s.exe, agent, s.lookup, runAgentCommand)
	}
	return s, nil
}

func setup(ctx context.Context, baseURL string, args []string) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	agent := flags.String("agent", "auto", "agent to connect (automatically detected by default)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: skillgild setup [--agent claude-code|codex|cursor|gemini-cli|antigravity|antigravity-cli]")
	}
	s, err := newOnboarding()
	if err != nil {
		return err
	}
	selected, err := s.selectAgent(*agent)
	if err != nil {
		return err
	}
	return s.prepare(ctx, baseURL, selected)
}

func (s *onboarding) detectedAgents() []setupAgent {
	var found []setupAgent
	for _, agent := range setupAgents {
		_, binaryErr := s.lookup(agent.binary)
		directoryFound := false
		if agent.directory != "" {
			info, err := os.Stat(filepath.Join(s.home, agent.directory))
			directoryFound = err == nil && info.IsDir()
		}
		// Codex registration requires its CLI; a leftover config directory alone
		// is not enough. Gemini's directory is also shared with Antigravity.
		if binaryErr == nil || (directoryFound && agent.id != "codex" && agent.id != "claude-code") {
			found = append(found, agent)
		}
	}
	return found
}

func (s *onboarding) selectAgent(requested string) (string, error) {
	if requested != "auto" {
		for _, agent := range setupAgents {
			if agent.id == requested {
				if agent.id == "codex" || agent.id == "claude-code" || agent.id == "gemini-cli" {
					if _, err := s.lookup(agent.binary); err != nil {
						return "", fmt.Errorf("%s CLI was not found on PATH; install it or choose another --agent", agent.name)
					}
				}
				return requested, nil
			}
		}
		return "", fmt.Errorf("unsupported setup agent %q; use claude-code, codex, cursor, gemini-cli, antigravity or antigravity-cli", requested)
	}
	found := s.detectedAgents()
	// Prefer the agent explicitly selected on a previous successful setup.
	var previous struct {
		Agent string `json:"agent"`
	}
	if raw, err := os.ReadFile(s.preferencePath); err == nil && json.Unmarshal(raw, &previous) == nil {
		for _, agent := range found {
			if agent.id == previous.Agent {
				return agent.id, nil
			}
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no supported agent detected; install an agent, or run skillgild setup --agent cursor (or another supported agent)")
	}
	if len(found) == 1 {
		return s.selectAgent(found[0].id)
	}
	if !s.interactive {
		return "", fmt.Errorf("multiple agents detected; choose one with --agent (for example --agent %s); no configuration was changed", found[0].id)
	}
	fmt.Fprintln(s.out, "Which agent should SkillGild connect?")
	for i, agent := range found {
		fmt.Fprintf(s.out, "  %d. %s\n", i+1, agent.name)
	}
	reader := bufio.NewReader(s.in)
	for {
		fmt.Fprint(s.out, "Agent number (or q to cancel): ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("agent selection cancelled; rerun with --agent to select it explicitly")
		}
		choice := strings.TrimSpace(line)
		if choice == "q" || choice == "quit" {
			return "", fmt.Errorf("setup cancelled; no configuration was changed")
		}
		if index, err := strconv.Atoi(choice); err == nil && index > 0 && index <= len(found) {
			return s.selectAgent(found[index-1].id)
		}
		for _, agent := range found {
			if choice == agent.id {
				return s.selectAgent(agent.id)
			}
		}
		fmt.Fprintln(s.out, "Choose one of the agents listed above.")
	}
}

func (s *onboarding) prepare(ctx context.Context, baseURL, agent string) error {
	if err := s.authenticate(ctx, baseURL, s.interactive); err != nil {
		return err
	}
	if err := s.register(ctx, agent); err != nil {
		return fmt.Errorf("sign-in is ready, but connecting %s failed: %w", agent, err)
	}
	raw, _ := json.Marshal(struct {
		Agent string `json:"agent"`
	}{agent})
	if err := writeSetupPreference(s.preferencePath, append(raw, '\n')); err != nil {
		fmt.Fprintf(s.out, "Connected, but could not remember the selected agent: %v\n", err)
	}
	fmt.Fprintf(s.out, "SkillGild is ready for %s. Restart or reload your agent to load the connection.\n", agent)
	return nil
}

func writeSetupPreference(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeConfigAtomically(path, raw, 0o600)
}

func ensureSetupLogin(ctx context.Context, baseURL string, interactive bool) error {
	return ensureSetupLoginWith(ctx, baseURL, interactive, agentclient.LoadAPIKey, login, strings.TrimSpace(os.Getenv(agentclient.EnvAPIKey)) != "")
}

func ensureSetupLoginWith(ctx context.Context, baseURL string, interactive bool, load func() (string, error), signIn func(context.Context, string) error, environmentKey bool) error {
	key, err := load()
	if err != nil && !errors.Is(err, agentclient.ErrNoCredential) {
		return err
	}
	if key != "" {
		api, err := agentclient.New(baseURL, key)
		if err != nil {
			return err
		}
		if _, err := api.Me(ctx); err == nil {
			return nil
		} else {
			var apiErr *agentclient.APIError
			if !errors.As(err, &apiErr) || apiErr.Status != 401 {
				return fmt.Errorf("could not verify your SkillGild sign-in: %w", err)
			}
			if environmentKey {
				return fmt.Errorf("SKILLGILD_API_KEY was rejected; replace it or unset it before signing in")
			}
		}
	}
	if !interactive {
		return fmt.Errorf("SkillGild sign-in is required; run skillgild setup in an interactive terminal, or set SKILLGILD_API_KEY for automation")
	}
	fmt.Println("Sign in to SkillGild once. Approve the device code in your browser to continue.")
	return signIn(ctx, baseURL)
}
