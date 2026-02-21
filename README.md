# Argus — AI Security Intelligence Agent

<div align="center">

**Autonomous · Local-First · Open-Source**

Argus is an AI-powered security scanner built with Go and Svelte 5. It acts as a local security researcher that maps, sniffs, and deeply analyzes codebases for vulnerabilities, hardcoded secrets, misconfigurations, and exploitable data flows — entirely on your machine.

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev/)
[![Svelte 5](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte)](https://svelte.dev/)
[![Version](https://img.shields.io/badge/version-0.2.0-brightgreen)](CHANGELOG.md)
[![Ollama](https://img.shields.io/badge/Powered%20by-Ollama-black)](https://ollama.com/)

📖 **[Full Project Wiki (powered by DeepWiki)](https://deepwiki.com/MeryylleA/argus-beta)**

</div>

---

## Overview

Argus runs a fully autonomous AI agent loop against any local codebase. The agent is given a sandboxed set of filesystem tools and iterates — reading files, searching patterns, tracing data flows — until it has exhausted all investigation paths. Every action and finding streams live to the browser via Server-Sent Events.

No cloud uploads. No API keys required. Your code stays on your machine.

---

## Features

| Feature | Description |
|---|---|
| 🔍 **Autonomous Recon & Exploit Modes** | Instruct the agent to map your architecture (Recon) or trace user inputs to dangerous sinks (Exploit). |
| ⚡ **Real-Time SSE Streaming** | Watch the agent's raw thoughts and tool calls stream live through a minimalist terminal UI. |
| 🛡️ **Junk Shield Sandbox** | Strictly blacklists directories (`node_modules`, `.git`, etc.) and enforces a 500 KB file read cap to prevent context-window flooding. |
| 🔗 **Autonomous Tool Chaining** | Dynamically chains `read_file`, `list_directory`, `search_code`, `grep_search`, `file_exists`, `report_finding`, `submit_summary`, and `update_memory` based on ongoing intelligence gathering. |
| 🧠 **Persistent Memory Whiteboard** | The agent writes key discoveries to a shared scratchpad (`update_memory`) that is injected into every subsequent LLM call, preventing amnesia across long sessions. |
| 🗜️ **Context Compaction** | When the conversation history exceeds 30 messages, "The Janitor" automatically trims it to the first message + the 10 most recent, preventing context-window overflow without losing critical state. |
| 🏠 **Local-First & Private** | Works out of the box with [Ollama](https://ollama.com/). Your code never leaves your machine. |
| 🧩 **Extensible Go Backend** | Clean, stateless HTTP API with a modular tool registry, an SSE broker with deadlock-safe non-blocking publishes, and ANSI-colored structured logging. |

---

## Architecture

```
argus/
├── cmd/
│   └── argus/
│       └── main.go               # Entry point — loads config, starts HTTP server
├── internal/
│   ├── api/
│   │   ├── server.go             # HTTP server, SSE broker, CORS middleware
│   │   └── handlers.go           # Route handlers (/scan, /sessions/{id}/stream, /health)
│   ├── agent/
│   │   ├── runner.go             # Core AI loop — tool execution, context compaction, system prompts
│   │   └── sandbox.go            # "Junk Shield" — path validation, file reads, GrepSearch
│   ├── llm/
│   │   └── provider.go           # Unified LLM interface + Ollama NDJSON stream parser
│   ├── logger/
│   │   └── logger.go             # ANSI-colored structured terminal logger
│   └── sse/
│       └── event.go              # Shared SSE event types (decouples api ↔ agent)
└── frontend/                     # Svelte 5 application
    └── src/
        ├── lib/stores/
        │   └── agent.svelte.ts   # Reactive Svelte 5 runes store for agent state
        ├── routes/
        │   ├── +page.svelte      # Main terminal UI
        │   └── FindingCard.svelte # Vulnerability finding component
        └── app.html
```

For a deep-dive into every module, data flow, and design decision, see the **[full project wiki on DeepWiki](https://deepwiki.com/MeryylleA/argus-beta)**.

---

## Getting Started

### Prerequisites

- [Go 1.24+](https://go.dev/doc/install)
- [Node.js 20+ & npm](https://nodejs.org/en)
- [Ollama](https://ollama.com/) *(recommended for local, private scans)*

### 1. Clone the Repository

```bash
git clone https://github.com/argus-sec/argus.git
cd argus
```

### 2. Start the Go Backend

The backend serves the REST API and SSE stream on `http://localhost:8080`.

```bash
# Install Go dependencies
go mod tidy

# Start the server
ARGUS_MODEL="ollama:minimax-m2.5:cloud" go run cmd/argus/main.go
```

> **Tip:** Set `ARGUS_MODEL` to any model available in your Ollama instance (e.g., `llama3`, `codellama`, `deepseek-coder`).

### 3. Start the Svelte Frontend

Open a **new terminal** and run:

```bash
cd frontend
npm install
npm run dev
```

The UI will be available at **`http://localhost:5173`**.

---

## Usage

1. **Enter a Target Path** — In the input box, type the **absolute path** to the repository you want to scan (e.g., `/home/user/projects/my-app`).
2. **Start the Scan** — Submit the form. The backend creates a sandboxed session and immediately begins streaming.
3. **Watch the Agent Work** — The terminal updates in real-time with the agent's current action while faint "hacker thoughts" stream in the background.
4. **Review Findings** — Vulnerabilities are surfaced as `FindingCard` components as they are discovered, classified by severity.
5. **Read the Report** — When the agent finishes, it submits an **Executive Summary** with an overall risk rating and a hypothetical attack chain, which is typed out character-by-character.

> **Easter Egg:** Try typing `do a barrel roll` in the target path input. 🙃

---

## API Reference

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/health` | Health check. Returns service name and version. |
| `POST` | `/api/scan` | Start a new scan session. Returns a `session_id`. |
| `GET` | `/api/sessions/{id}/stream` | SSE stream for a session. |

### POST /api/scan

**Request body:**

```json
{
  "target_path": "/absolute/path/to/repo",
  "role": "recon"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `target_path` | `string` | ✅ | Absolute path to the directory to scan. |
| `role` | `string` | ❌ | Agent role: `recon` (default) or `exploit`. |

**Response:**

```json
{ "session_id": "a3f8c2..." }
```

### SSE Event Types

Once connected to `/api/sessions/{id}/stream`, the following events are emitted:

| Event | Payload Fields | Description |
|---|---|---|
| `connected` | `session_id` | Emitted immediately on connection. |
| `session_start` | `session_id`, `agent_role`, `model` | Agent loop has started. |
| `thinking` | `iteration`, `delta` | Agent is processing; iteration counter or thinking delta. |
| `thought` | `delta` | Raw LLM text output delta. |
| `tool_call` | `tool` | Agent is about to invoke a tool. |
| `tool_result` | `tool`, `result` | Truncated result of a tool call. |
| `tool_error` | `tool`, `error` | A tool call failed. |
| `finding_reported` | `title`, `severity`, `file`, `desc` | A vulnerability finding was submitted. |
| `scan_summary` | `overall_risk`, `summary`, `attack_chain` | Final executive summary. |
| `completed` | `message` | Agent loop finished. |
| `error` | `error` | Fatal agent error. |
| `session_end` | — | SSE channel closed. |

---

## Configuration

Argus is configured entirely through environment variables — no config files needed.

| Variable | Default | Description |
|---|---|---|
| `ARGUS_MODEL` | `minimax-m2.5:cloud` | Ollama model identifier for both Recon and Exploit roles. |
| `ARGUS_LISTEN_ADDR` | `:8080` | Host and port for the Go HTTP server. |
| `OLLAMA_HOST` | `http://localhost:11434` | Base URL for the Ollama API instance. |

---

## The Junk Shield Sandbox

Argus operates inside a strict filesystem sandbox to prevent runaway reads and LLM context flooding. All file operations are validated through the `Sandbox` type in `internal/agent/sandbox.go`.

**Security guarantees:**
- Absolute paths are rejected — all access must be relative to the workspace root.
- Path traversal (`../`) is blocked at the string and filesystem level.
- Symlink escapes are caught by resolving all paths with `filepath.EvalSymlinks`.

**Blacklisted directories** (automatically skipped during traversal):

`.git`, `node_modules`, `venv`, `.venv`, `env`, `__pycache__`, `dist`, `build`, `vendor`, `.idea`, `.vscode`, `coverage`

**Blacklisted file extensions** (binary & media files):

`.exe`, `.dll`, `.so`, `.png`, `.jpg`, `.jpeg`, `.gif`, `.pdf`, `.zip`, `.tar`, `.gz`, `.mp4`, `.mp3`, `.wav`, `.ico`, `.svg`, `.woff`, `.ttf`

**Hard limits:**

| Limit | Value |
|---|---|
| Max file read size | 500 KB |
| Max `grep_search` matches | 50 |
| Max `search_code` matches | 100 |
| Max agent iterations | 50 |
| Context compaction threshold | 30 messages (keeps first + last 10) |

---

## Agent Tools

The agent has access to the following sandboxed tools:

| Tool | Description |
|---|---|
| `read_file` | Read a file's contents (relative path, ≤ 500 KB). |
| `list_directory` | List directory contents with names, types, and sizes. |
| `file_exists` | Check whether a file or directory exists. |
| `search_code` | Case-sensitive substring search across source files; returns file path and line number. |
| `grep_search` | Regex or string search across the entire codebase (up to 50 matches). |
| `report_finding` | Submit a verified vulnerability finding with title, severity, description, file path, and evidence. |
| `submit_summary` | Submit the final executive summary with overall risk and attack chain. Called exactly once at the end. |
| `update_memory` | Append text to the persistent whiteboard. Content is injected into every subsequent LLM system prompt. |

---

## Roadmap

- [ ] Automated Git Cloning — scan repositories directly from URLs.
- [ ] Multi-agent Workflows — Recon passes findings to the Exploit agent.
- [ ] Expose Exploit mode in the frontend UI.
- [ ] PDF & HTML Report Export.
- [ ] Granular finding severity scoring.
- [ ] Per-session rate limiting and authentication.

---

## Documentation

For a complete, in-depth reference covering architecture, API endpoints, agent internals, and the LLM provider interface, visit the community wiki:

**👉 [deepwiki.com/MeryylleA/argus-beta](https://deepwiki.com/MeryylleA/argus-beta)**

---

## License

This project is open-source and available under the [MIT License](LICENSE).

---

<div align="center">
  Built with ❤️ by <a href="https://github.com/MeryylleA">MeryylleA</a>
</div>
