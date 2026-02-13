# Argus
[![Go Version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![CI](https://github.com/MeryylleA/argus-beta/actions/workflows/ci.yml/badge.svg)](https://github.com/MeryylleA/argus-beta/actions/workflows/ci.yml)

Argus is a local-first, open-source security intelligence platform for bug bounty research. It runs on your machine, uses your own API keys, and keeps investigation context between sessions so agents can avoid repeating work.

Argus focuses on autonomous repository investigation with strict defensive controls:
- Read-only tool execution
- Path-restricted sandboxing
- Symlink escape prevention
- Auditable tool activity logs

## Why Argus
Argus differs from general-purpose coding assistants by prioritizing security research workflows:
1. **Project memory** with SQLite-backed findings and investigated areas
2. **Bug bounty context injection** (scope, rules, reward ranges)
3. **Security-first sandbox** (read-only and path-restricted tools)
4. **Multi-provider support** via user-provided API keys
5. **Collaborative mode** with parallel specialized agents and a shared channel

## Architecture
```text
+------------------+       +--------------------+       +---------------------+
| Researcher / CLI | ----> | Argus Agent Runner | ----> | Tool Executor       |
+------------------+       +--------------------+       +---------------------+
                                                             |
                                                             v
                                                     +------------------+
                                                     | Sandbox Guard    |
                                                     | - path checks    |
                                                     | - symlink checks |
                                                     | - read-only exec |
                                                     +------------------+
                                                             |
                                                             v
                                        +---------------------------------------------+
                                        | Tools                                        |
                                        | search_code / view_lines / find_files       |
                                        | directory_tree / git_log                    |
                                        +---------------------------------------------+
```

## Installation
### Prerequisites
- Go `1.22+`
- `rg` (ripgrep)
- `fd` (preferred) or `find`
- `git`

### Build
```bash
git clone https://github.com/MeryylleA/argus-beta.git
cd argus-beta
go build -o bin/argus ./cmd/argus
```

### Quick Start
```bash
./bin/argus tool search_code "(auth|token|password|secret)" . 20
./bin/argus tool view_lines internal/core/tools/sandbox.go 1 80
./bin/argus tool find_files . "*.go" go
./bin/argus tool directory_tree . 3
./bin/argus tool git_log . 10
```

## CLI Usage
### `search_code`
Searches for regex patterns in a path with a max result limit.
```bash
argus tool search_code "(jwt|apikey|secret)" ./internal 50
```

### `view_lines`
Views bounded line ranges from a file.
```bash
argus tool view_lines internal/core/tools/executor.go 1 120
```

### `find_files`
Finds files by pattern/extension using `fd` (fallback to `find`).
```bash
argus tool find_files . "*.go" go
```

### `directory_tree`
Prints repository tree to a max depth.
```bash
argus tool directory_tree . 2
```

### `git_log`
Shows recent commit history for a path.
```bash
argus tool git_log . 15
```

## Configuration (Phase 2 Preview)
Argus Phase 1 runs tool execution and sandboxing. Phase 2 introduces provider adapters, agent loop execution, and SQLite project memory.

Set provider credentials as environment variables:
- `ANTHROPIC_API_KEY`
- `OPENROUTER_API_KEY`
- `OPENAI_API_KEY`
- `GEMINI_API_KEY`

Recommended local settings:
- `ARGUS_DB_PATH=.argus/argus.db`
- `ARGUS_PROJECT_ROOT=/absolute/path/to/target/repo`

Module path note: `go.mod` uses `github.com/argus-sec/argus`, which may differ from the GitHub repository URL.

## Roadmap
| Phase | Scope | Status |
| --- | --- | --- |
| 1 | Tool Executor + Sandbox | ✅ Complete |
| 2 | Provider adapters + Agent runner + SQLite memory | 🔄 In progress |
| 3 | Agent orchestrator + SQLite schema | 📋 Planned |
| 4 | Collaborative mode (2 agents + shared channel) | 📋 Planned |
| 5 | HTTP API + SSE/WebSocket streaming | 📋 Planned |
| 6 | Svelte frontend | 📋 Planned |

## Contributing
Argus is a solo-maintained project designed for community contributions. See `CONTRIBUTING.md` for local setup, standards, and PR process.

## Security Disclosure Policy
If you discover a vulnerability in Argus itself, report it privately through GitHub Security Advisories. Do not open public issues for vulnerabilities. See `SECURITY.md` for response timelines and scope.

## License
Argus is licensed under the MIT License.
