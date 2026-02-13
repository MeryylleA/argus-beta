# Prompt: Generate Argus Repository Documentation & Infrastructure Files

You are building the complete documentation and repository infrastructure for **Argus**, an open-source AI-powered security intelligence platform for bug bounty research.

## Context: What Argus Is

Argus is a **local-first, open-source** tool that runs on the researcher's machine. It uses AI agents to autonomously investigate open-source repositories and discover vulnerabilities. The researcher provides their own API keys — no telemetry, no SaaS, fully auditable.

**GitHub:** https://github.com/MeryylleA/argus-beta  
**Current Phase:** Phase 1 complete (tool executor + sandbox)

### Core Differentiators from Claude Code / other AI tools
1. **Project Memory** — SQLite-backed findings, investigated areas, deduplication across sessions
2. **Bug Bounty Context** — program scope, rules, reward ranges injected into every agent session
3. **Security Sandbox** — all tools read-only, path-restricted, no code execution
4. **Multi-provider** — Anthropic, OpenRouter, OpenAI, Gemini via user's own API keys
5. **Collaborative Mode** — two specialized agents investigate in parallel with a shared channel

### Tech Stack
- **Backend:** Go 1.22+ (single binary, no runtime dependencies)
- **Frontend:** TypeScript + Svelte (planned Phase 6)
- **Communication:** WebSockets (planned Phase 3+)
- **Database:** SQLite via modernc.org/sqlite (no CGO)
- **Styling:** CSS Pure + Monospace fonts (JetBrains Mono / Fira Code)
- **LLM:** REST APIs — Anthropic, OpenRouter, OpenAI, Gemini

### Project Structure (current)
```
argus/
├── cmd/argus/main.go
├── internal/
│   ├── core/
│   │   ├── tools/             ← Phase 1 COMPLETE
│   │   │   ├── types.go
│   │   │   ├── sandbox.go     ← EvalSymlinks + prefix check
│   │   │   ├── logger.go      ← audit trail, writes to stderr
│   │   │   ├── executor.go    ← 30s timeout + goroutine
│   │   │   ├── params.go
│   │   │   ├── tool_search_code.go   ← ripgrep --no-follow
│   │   │   ├── tool_view_lines.go    ← max 100 lines
│   │   │   ├── tool_find_files.go    ← fd/find fallback
│   │   │   ├── tool_directory_tree.go ← pure Go
│   │   │   ├── tool_git_log.go       ← GIT_CEILING_DIRECTORIES
│   │   │   ├── sandbox_test.go
│   │   │   ├── executor_test.go
│   │   │   ├── tool_view_lines_test.go
│   │   │   └── tool_directory_tree_test.go
│   │   ├── agent/             ← Phase 2 (planned)
│   │   ├── channel/           ← Phase 3 (planned)
│   │   └── memory/            ← Phase 2 (planned)
│   └── providers/             ← Phase 2 (planned)
├── api/                       ← Phase 5 (planned)
├── go.mod
└── go.sum
```

### Development Roadmap
- **Phase 1** ✅ Tool Executor + Sandbox (complete)
- **Phase 2** 🔄 Provider adapters (Anthropic/OpenRouter/Gemini) + Agent runner loop + SQLite memory
- **Phase 3** Agent orchestrator + SQLite schema (projects, sessions, findings, investigated_areas)
- **Phase 4** Collaborative mode (2 agents + shared channel)
- **Phase 5** HTTP API + SSE/WebSocket streaming
- **Phase 6** Svelte frontend

### Tool Interface (for documentation reference)
```go
type Tool interface {
    Name() string
    Description() string
    Schema() ToolSchema   // JSON schema sent to LLM providers
    Execute(params map[string]any) (ToolResult, error)
}
```

### Available CLI Commands (Phase 1)
```bash
argus tool search_code <pattern> <path> [max_results]
argus tool view_lines <file> <start_line> <end_line>
argus tool find_files <path> [pattern] [extension]
argus tool directory_tree <path> [depth]
argus tool git_log <path> [count]
```

### External Dependencies
- `rg` (ripgrep) — for search_code
- `fd` (optional, preferred) or `find` — for find_files
- `git` — for git_log
- Go 1.22+ — for building

---

## Your Task

Generate ALL of the following files. Create each file immediately — do not ask for confirmation between files. Use the terminal to create them.

### Files to Create

#### 1. `README.md`
A professional, well-structured README with:
- Project description with the security intelligence angle
- Badges: Go version, license (MIT), build status (GitHub Actions)
- Architecture diagram (ASCII art showing agent → tool executor → sandbox → tools)
- Installation instructions (prerequisites + `go build` + quick start)
- CLI usage with real examples for each tool
- Configuration section (API keys, project setup for Phase 2)
- Roadmap table showing all 6 phases with status
- Contributing section
- Security disclosure policy (responsible disclosure, no public issues for vulns)
- License section

#### 2. `CONTRIBUTING.md`
- How to set up the development environment
- Code standards: Go fmt, vet, staticcheck
- Testing requirements: all security tests must pass, new tools need sandbox tests
- PR process: conventional commits (feat/fix/sec/docs/test)
- Security-specific contribution guidelines (no test that actually exploits)
- Branch strategy: main (stable), develop, feature/*, fix/*, sec/*

#### 3. `SECURITY.md`
- Security policy: supported versions
- How to report a vulnerability (private disclosure via GitHub Security Advisories)
- What qualifies as a security issue in Argus itself
- Response timeline: acknowledge in 48h, patch in 7 days for critical
- Out of scope: findings in repos that users analyze (those are intended)
- Note: Argus is a security tool — issues in the sandbox are critical priority

#### 4. `CHANGELOG.md`
- Format: Keep a Changelog (https://keepachangelog.com)
- Version: 0.1.0 (Phase 1)
- Added: all 5 tools, sandbox with EvalSymlinks, audit logger, CLI
- Security: path traversal prevention, symlink escape detection, subprocess isolation

#### 5. `.github/ISSUE_TEMPLATE/bug_report.md`
Standard bug report template with:
- Description, reproduction steps, expected vs actual behavior
- Go version, OS, relevant tool name
- Checkbox: "I have confirmed this is not a security vulnerability"
- Note directing security issues to SECURITY.md

#### 6. `.github/ISSUE_TEMPLATE/feature_request.md`
Feature request template with:
- Problem description, proposed solution
- Which phase this aligns with (1-6)
- Checkboxes: affects sandbox security, requires new external dependency

#### 7. `.github/PULL_REQUEST_TEMPLATE.md`
PR template with:
- Type of change (feat/fix/sec/docs/refactor/test)
- Description of changes
- Security checklist: sandbox tests pass, no new symlink risks, no stdout log pollution
- Testing checklist: `go test ./...`, new tests added, manual CLI test
- Phase this PR belongs to

#### 8. `.github/workflows/ci.yml`
GitHub Actions CI pipeline:
- Trigger: push to main/develop, all PRs
- Go version matrix: 1.22, 1.23
- OS matrix: ubuntu-latest, macos-latest
- Steps: checkout, setup-go, go vet, go build, go test with -race flag
- Install ripgrep and fd for tool integration tests
- Upload test coverage to Codecov (optional, with if: condition)
- Cache go modules

#### 9. `.github/workflows/security.yml`
Security-focused workflow:
- Trigger: push to main, weekly schedule (cron)
- Steps: govulncheck (Go vulnerability database check)
- gosec static analysis
- Fail on high severity findings
- Upload SARIF results to GitHub Security tab

#### 10. `docs/architecture.md`
Technical architecture document:
- Security model explanation (sandbox threat model)
- Tool execution flow diagram (ASCII)
- Data flow: LLM → tool_call JSON → executor → sandbox validation → tool → result
- Concurrency model: goroutines for timeout, mutex in logger
- Why each design decision was made (Go over Python, SQLite over Postgres, etc.)
- Phase 2 architecture preview: provider interface, agent loop pseudocode

#### 11. `docs/tools.md`
Tool reference documentation:
- Each tool: purpose, parameters, example input/output, security constraints
- search_code: include example of finding auth patterns in a Go repo
- view_lines: include example output format (`   1 | package main`)
- find_files: fd vs find differences
- directory_tree: show example tree output, explain skipped directories
- git_log: explain GIT_CEILING_DIRECTORIES security feature

#### 12. `docs/security-model.md`
Deep dive into Argus's security model:
- Why a security tool needs its own security model
- Threat model: what happens if the analyzed repo is malicious
- Sandbox layers (4 layers of defense in depth)
- Path validation algorithm step by step
- Symlink attack scenarios and how each is prevented
- Subprocess isolation: minimal environment, --no-follow flags
- Audit trail: what gets logged and why
- What Argus deliberately does NOT do (no code execution, no writes, no network from tools)

#### 13. `.golangci.yml`
golangci-lint configuration:
- Enabled linters: govet, errcheck, staticcheck, gosec, godot, misspell
- Excluded paths: _test.go files for some linters
- Settings: gosec should check for path traversal (G304, G305)
- Max issues per linter: 0 (fail on any)

#### 14. `Makefile`
Common development commands:
- `make build` — go build
- `make test` — go test ./... -race -v
- `make test-security` — run only sandbox/security tests
- `make lint` — golangci-lint run
- `make fmt` — go fmt + goimports
- `make vuln` — govulncheck ./...
- `make clean` — remove build artifacts
- `make install-deps` — install rg, fd, git check
- `make` (default) — fmt + lint + test

---

## Style Guidelines for All Files

- Use present tense ("Argus investigates" not "Argus will investigate")
- Be direct and technical — the audience is security researchers and Go developers
- ASCII diagrams where helpful, not mandatory everywhere
- Markdown: use code blocks with language hints, use tables for structured data
- Tone: professional but not corporate. This is a hacker tool for hackers.
- Do NOT add emojis everywhere. Use them sparingly (roadmap table: ✅ 🔄 📋 is fine)
- Every security-relevant decision should have a "why" explanation

## Important Notes

- The GitHub URL is https://github.com/MeryylleA/argus-beta
- Module path in go.mod is `github.com/argus-sec/argus` (may differ from repo URL — note this)
- This is a solo project currently, but written to accept contributions
- Target audience: bug bounty researchers, security engineers, Go developers
- License: MIT

Start with README.md, then create each file in order. Use `mkdir -p` for directories before creating files inside them.
