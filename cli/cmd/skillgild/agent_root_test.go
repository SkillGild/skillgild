package main

import (
	"path/filepath"
	"testing"
)

func TestAgentSkillRoot(t *testing.T) {
	home := filepath.FromSlash("/home/dev")
	for agent, want := range map[string]string{
		"shared":          ".agents/skills",
		"codex":           ".agents/skills",
		"cursor":          ".cursor/skills",
		"claude-code":     ".claude/skills",
		"gemini-cli":      ".gemini/skills",
		"antigravity":     ".gemini/config/skills",
		"antigravity-cli": ".gemini/antigravity-cli/skills",
	} {
		got, ok := agentSkillRoot(home, agent)
		if !ok || got != filepath.Join(home, filepath.FromSlash(want)) {
			t.Errorf("%s: got %q, %v", agent, got, ok)
		}
	}
	if _, ok := agentSkillRoot(home, "emacs"); ok {
		t.Error("unknown preset accepted")
	}
}
