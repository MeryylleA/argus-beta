package agent

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptMinimal(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "my-app",
		RootPath:    "/src/my-app",
	})

	mustContain := []string{
		"senior security researcher",
		"## Project: my-app",
		"Root path: /src/my-app",
		"## Investigation Methodology",
		"## Reporting Findings",
	}
	for _, s := range mustContain {
		if !strings.Contains(prompt, s) {
			t.Errorf("prompt missing %q", s)
		}
	}

	// Single mode should not mention collaborative
	if strings.Contains(prompt, "Collaborative Mode") {
		t.Error("single mode prompt should not contain collaborative section")
	}
}

func TestBuildSystemPromptFocus(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
		Focus:       "authentication bypass",
	})

	if !strings.Contains(prompt, "## Investigation Focus") {
		t.Error("prompt missing Investigation Focus heading")
	}
	if !strings.Contains(prompt, "authentication bypass") {
		t.Error("prompt missing focus text")
	}
}

func TestBuildSystemPromptNoFocus(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
	})

	if strings.Contains(prompt, "## Investigation Focus") {
		t.Error("prompt should not contain Investigation Focus when Focus is empty")
	}
}

func TestBuildSystemPromptScope(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
		Scope: ScopeConfig{
			InScope:  []string{"src/auth/", "src/api/"},
			OutScope: []string{"vendor/", "tests/"},
		},
	})

	if !strings.Contains(prompt, "## Scope") {
		t.Error("prompt missing Scope heading")
	}
	if !strings.Contains(prompt, "IN SCOPE") {
		t.Error("prompt missing IN SCOPE section")
	}
	if !strings.Contains(prompt, "src/auth/") {
		t.Error("prompt missing in-scope path")
	}
	if !strings.Contains(prompt, "OUT OF SCOPE") {
		t.Error("prompt missing OUT OF SCOPE section")
	}
	if !strings.Contains(prompt, "vendor/") {
		t.Error("prompt missing out-of-scope path")
	}
}

func TestBuildSystemPromptBountyProgram(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
		BountyProgram: &BountyProgram{
			Platform:    "hackerone",
			Name:        "Acme Corp",
			Rules:       "No social engineering",
			RewardRange: "$100-$10000",
		},
	})

	if !strings.Contains(prompt, "## Bug Bounty Program: Acme Corp (hackerone)") {
		t.Error("prompt missing bounty program header")
	}
	if !strings.Contains(prompt, "$100-$10000") {
		t.Error("prompt missing reward range")
	}
	if !strings.Contains(prompt, "No social engineering") {
		t.Error("prompt missing program rules")
	}
}

func TestBuildSystemPromptPreviousFindings(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:             "single",
		ProjectName:      "app",
		RootPath:         "/app",
		PreviousFindings: []string{"SQLi in login", "XSS in profile"},
	})

	if !strings.Contains(prompt, "## Already Reported Findings") {
		t.Error("prompt missing previous findings heading")
	}
	if !strings.Contains(prompt, "Do NOT report duplicates") {
		t.Error("prompt missing dedup instruction")
	}
	if !strings.Contains(prompt, "SQLi in login") {
		t.Error("prompt missing first finding")
	}
	if !strings.Contains(prompt, "XSS in profile") {
		t.Error("prompt missing second finding")
	}
}

func TestBuildSystemPromptNoPreviousFindings(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
	})

	if strings.Contains(prompt, "Already Reported") {
		t.Error("prompt should not contain findings section when list is empty")
	}
}

func TestBuildSystemPromptInvestigatedAreas(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:              "single",
		ProjectName:       "app",
		RootPath:          "/app",
		InvestigatedAreas: []string{"auth/: SQL patterns", "api/: IDOR checks"},
	})

	if !strings.Contains(prompt, "## Previously Investigated Areas") {
		t.Error("prompt missing investigated areas heading")
	}
	if !strings.Contains(prompt, "auth/: SQL patterns") {
		t.Error("prompt missing first investigated area")
	}
}

func TestBuildSystemPromptCollaborativeAgentA(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:         "agent_a",
		ProjectName:  "app",
		RootPath:     "/app",
		PartnerModel: "gpt-5.2",
	})

	if !strings.Contains(prompt, "## Collaborative Mode") {
		t.Error("prompt missing collaborative mode section")
	}
	if !strings.Contains(prompt, "You are Agent A") {
		t.Error("prompt should identify as Agent A")
	}
	if !strings.Contains(prompt, "gpt-5.2") {
		t.Error("prompt should mention partner model")
	}
	if !strings.Contains(prompt, "read_channel") {
		t.Error("prompt should mention read_channel tool")
	}
	if !strings.Contains(prompt, "post_channel") {
		t.Error("prompt should mention post_channel tool")
	}
}

func TestBuildSystemPromptCollaborativeAgentB(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:         "agent_b",
		ProjectName:  "app",
		RootPath:     "/app",
		PartnerModel: "claude-opus-4-6",
	})

	if !strings.Contains(prompt, "You are Agent B") {
		t.Error("prompt should identify as Agent B")
	}
	if !strings.Contains(prompt, "Agent A") {
		t.Error("prompt should mention partner Agent A")
	}
}

func TestBuildSystemPromptMethodologySections(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
	})

	// Check key methodology steps exist
	steps := []string{
		"directory_tree",
		"find_files",
		"search_code",
		"view_lines",
		"git_log",
		"mark_investigated",
	}
	for _, step := range steps {
		if !strings.Contains(prompt, step) {
			t.Errorf("methodology missing reference to %q", step)
		}
	}
}

func TestBuildSystemPromptReportingInstructions(t *testing.T) {
	prompt := BuildSystemPrompt(PromptConfig{
		Mode:        "single",
		ProjectName: "app",
		RootPath:    "/app",
	})

	fields := []string{"title", "location", "severity", "confidence", "description", "data_flow", "category"}
	for _, f := range fields {
		if !strings.Contains(prompt, f) {
			t.Errorf("reporting section missing field %q", f)
		}
	}
}
