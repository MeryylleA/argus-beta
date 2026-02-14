package agent

import (
	"fmt"
	"strings"
)

// PromptConfig holds everything needed to build the system prompt.
type PromptConfig struct {
	Mode              string         // "single" | "agent_a" | "agent_b"
	ProjectName       string
	RootPath          string
	Focus             string         // what to look for (e.g., "authentication bypass")
	Scope             ScopeConfig
	BountyProgram     *BountyProgram // optional
	PreviousFindings  []string       // titles of findings from previous sessions
	InvestigatedAreas []string       // "path: pattern" strings already covered
	PartnerModel      string         // only in collaborative mode
}

// ScopeConfig defines paths in and out of scope.
type ScopeConfig struct {
	InScope  []string // paths/packages to investigate
	OutScope []string // paths/packages to skip
}

// BountyProgram contains optional bug bounty metadata.
type BountyProgram struct {
	Platform    string // "hackerone" | "bugcrowd" | "immunefi"
	Name        string
	Rules       string
	RewardRange string
}

// BuildSystemPrompt constructs the full system prompt for an agent.
func BuildSystemPrompt(cfg PromptConfig) string {
	var b strings.Builder

	// 1. Role identification
	b.WriteString("You are a senior security researcher conducting a thorough vulnerability assessment.\n")
	b.WriteString("Your goal is to find real, exploitable security vulnerabilities — not theoretical issues.\n")
	b.WriteString("You have access to the project's source code through a set of tools.\n\n")

	// 2. Project context
	b.WriteString(fmt.Sprintf("## Project: %s\n", cfg.ProjectName))
	b.WriteString(fmt.Sprintf("Root path: %s\n\n", cfg.RootPath))

	// 3. Investigation focus
	if cfg.Focus != "" {
		b.WriteString(fmt.Sprintf("## Investigation Focus\n%s\n\n", cfg.Focus))
	}

	// 4. Scope
	if len(cfg.Scope.InScope) > 0 || len(cfg.Scope.OutScope) > 0 {
		b.WriteString("## Scope\n")
		if len(cfg.Scope.InScope) > 0 {
			b.WriteString("IN SCOPE (investigate these):\n")
			for _, p := range cfg.Scope.InScope {
				b.WriteString(fmt.Sprintf("  - %s\n", p))
			}
		}
		if len(cfg.Scope.OutScope) > 0 {
			b.WriteString("OUT OF SCOPE (do not investigate):\n")
			for _, p := range cfg.Scope.OutScope {
				b.WriteString(fmt.Sprintf("  - %s\n", p))
			}
		}
		b.WriteString("\n")
	}

	// 5. Bug bounty program rules
	if cfg.BountyProgram != nil {
		bp := cfg.BountyProgram
		b.WriteString(fmt.Sprintf("## Bug Bounty Program: %s (%s)\n", bp.Name, bp.Platform))
		if bp.RewardRange != "" {
			b.WriteString(fmt.Sprintf("Reward range: %s\n", bp.RewardRange))
		}
		if bp.Rules != "" {
			b.WriteString(fmt.Sprintf("Program rules:\n%s\n", bp.Rules))
		}
		b.WriteString("\n")
	}

	// 6. Previous findings (deduplication)
	if len(cfg.PreviousFindings) > 0 {
		b.WriteString("## Already Reported Findings\n")
		b.WriteString("The following vulnerabilities have already been found. Do NOT report duplicates:\n")
		for _, f := range cfg.PreviousFindings {
			b.WriteString(fmt.Sprintf("  - %s\n", f))
		}
		b.WriteString("\n")
	}

	// 7. Investigated areas (avoid redundant work)
	if len(cfg.InvestigatedAreas) > 0 {
		b.WriteString("## Previously Investigated Areas\n")
		b.WriteString("These areas have already been analyzed. Focus on unexplored code:\n")
		for _, a := range cfg.InvestigatedAreas {
			b.WriteString(fmt.Sprintf("  - %s\n", a))
		}
		b.WriteString("\n")
	}

	// 8. Collaborative mode instructions
	if cfg.Mode == "agent_a" || cfg.Mode == "agent_b" {
		b.WriteString("## Collaborative Mode\n")
		if cfg.Mode == "agent_a" {
			b.WriteString("You are Agent A. You have a partner (Agent B) investigating complementary areas.\n")
		} else {
			b.WriteString("You are Agent B. You have a partner (Agent A) investigating complementary areas.\n")
		}
		if cfg.PartnerModel != "" {
			b.WriteString(fmt.Sprintf("Your partner is using model: %s\n", cfg.PartnerModel))
		}
		b.WriteString(`
Communication channel:
- Use read_channel every ~8 tool calls to check for messages from your partner.
- Use post_channel to share important context or findings with your partner.
- Message types: "finding" (share a discovered vulnerability), "question" (ask your partner),
  "context" (share useful context), "duplicate" (flag a duplicate finding).
- When you find something, BOTH post to channel AND call record_finding.
`)
		b.WriteString("\n")
	}

	// 9. Methodology instructions
	b.WriteString(`## Investigation Methodology
1. Start with directory_tree to understand the project structure.
2. Use find_files to locate key files (configs, auth modules, input handlers, etc.).
3. Use search_code for targeted pattern searches (e.g., SQL queries, eval(), exec(), unsafe deserialization).
4. Use view_lines to read specific code sections and confirm vulnerabilities.
5. Use git_log to check recent changes that might introduce bugs.
6. After investigating an area, call mark_investigated so it's not re-covered.

Be methodical. Map the attack surface before diving into specific files.
`)

	// 10. Reporting instructions
	b.WriteString(`
## Reporting Findings
When you confirm a vulnerability, call record_finding with:
- title: Clear, descriptive title
- location: file:line or file:function
- severity: critical | high | medium | low | info
- confidence: confirmed | likely | suspected
- description: What the vulnerability is and why it matters
- data_flow: How to trigger it (attack vector / data flow)
- category: CWE ID or custom category (e.g., "CWE-89: SQL Injection")

Only report real vulnerabilities you can trace through the code. Avoid false positives.
Do not report style issues, missing best practices, or theoretical risks without evidence.
`)

	return b.String()
}
