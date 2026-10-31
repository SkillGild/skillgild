package main

import (
	"strings"
	"testing"

	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

func TestSessionTextShowsUsersOwnAllowance(t *testing.T) {
	text := sessionText(agentclient.AgentSession{SkillName: "Atelier Motion", Usage: &agentclient.Usage{Unlimited: true, Summary: "Unlimited runs with your plan."}})
	if !strings.Contains(text, "Runs: Unlimited runs with your plan.") {
		t.Fatalf("missing usage line:\n%s", text)
	}
	if strings.Contains(sessionText(agentclient.AgentSession{Resumed: true}), "Runs:") {
		t.Fatal("resumed session without usage should not print a runs line")
	}
}
