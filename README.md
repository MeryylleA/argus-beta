# 👁️ Argus — AI Security Intelligence Agent

<div align="center">

**Autonomous. Local-first. Open-source.**

Argus is an AI-powered security scanner built with Go and Svelte 5. It acts as a local security researcher that maps, sniffs, and deeply analyzes codebases for vulnerabilities, hardcoded secrets, misconfigurations, and exploitable flows — entirely on your machine.

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go 1.23+](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Svelte 5](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte)](https://svelte.dev/)
[![Ollama](https://img.shields.io/badge/Powered%20by-Ollama-black)](https://ollama.com/)

📖 **[Full Project Wiki (powered by Devin AI)](https://deepwiki.com/MeryylleA/argus-beta)**

</div>

---

## ✨ Features

| Feature | Description |
|---|---|
| 🔍 **Autonomous Recon & Exploit Modes** | Instruct the agent to map your architecture (Recon) or trace user inputs to dangerous sinks (Exploit). |
| ⚡ **Real-time SSE Streaming** | Watch the agent's raw "thoughts" and tool calls stream live through a Cyberpunk-themed terminal UI. |
| 🛡️ **Junk Shield Sandbox** | Strictly blacklists directories (`node_modules`, `.git`, etc.) and enforces a 500KB file read cap to prevent context-window flooding. |
| 🔗 **Autonomous Tool Chaining** | Dynamically chains `read_file`, `list_directory`, `search_code`, `grep_search`, `git_blame`, and more based on ongoing intelligence gathering. |
| 🏠 **Local-First & Private** | Works out of the box with [Ollama](https://ollama.com/). Your code never leaves your machine. |
| 🧩 **Extensible Go Backend** | Clean, stateless HTTP API with modular tool registries and a Server-Sent Events hub. |

---

## 🏗️ Architecture

```
argus-beta/
├── cmd/
│   └── argus/
│       └── main.go           # Entry point — loads config, starts HTTP server
├── internal/
│   ├── api/
│   │   ├── server.go         # HTTP server, SSE broker, CORS middleware
│   │   └── handlers.go       # Route handlers (/scan, /stream, /health)
│   ├── agent/
│   │   ├── runner.go         # Core AI loop — tool execution & system prompts
│   │   └── sandbox.go        # "Junk Shield" — path validation, file reads, GrepSearch
│   ├── llm/
│   │   └── provider.go       # Unified LLM interface + Ollama NDJSON stream parser
│   └── sse/
│       └── event.go          # Shared SSE event types (decouples api ↔ agent)
└── frontend/                 # Svelte 5 application
    ├── src/
    │   ├── agent.svelte.ts   # Reactive store for agent state
    │   └── +page.svelte      # Main Cyberpunk terminal UI
    └── FindingCard.svelte    # Vulnerability finding component
```

For a deep-dive into every module, data flow, and design decision, check out the **[full project wiki on DeepWiki](https://deepwiki.com/MeryylleA/argus-beta)**.

---

## 🚀 Getting Started

### Prerequisites

- [Go 1.23+](https://go.dev/doc/install)
- [Node.js 20+ & npm](https://nodejs.org/en)
- [Ollama](https://ollama.com/) *(recommended for local, private scans)*

### 1. Clone the Repository

```bash
git clone https://github.com/MeryylleA/argus-beta.git
cd argus-beta
```

### 2. Start the Go Backend

The backend serves the REST API and SSE stream on `http://localhost:8080`.

```bash
# Install Go dependencies
go mod tidy

# Start the server (using a cloud model via Ollama)
ARGUS_MODEL="ollama:minimax-m2.5:cloud" go run cmd/argus/main.go
```

> **Tip:** Set the `ARGUS_MODEL` environment variable to match any model you have available in Ollama (e.g., `llama3`, `codellama`, `deepseek-coder`).

### 3. Start the Svelte Frontend

Open a **new terminal tab** and run:

```bash
cd argus-beta/frontend
npm install
npm run dev
```

The UI will be available at **`http://localhost:5173`**. Open it in your browser.

---

## 🛠️ Usage

1. **Enter a Target Path** — In the central input box, type the **absolute path** to the repository you want to scan (e.g., `/home/user/projects/my-app`).
2. **Choose a Mode** — Select **Recon** to map the architecture, or **Exploit** to trace dangerous data flows.
3. **Start the Scan** — Hit the button. The backend spawns a session and immediately begins streaming.
4. **Watch the Agent Work** — The terminal updates in real-time with the agent's actions (e.g., `Decrypting source code: src/main.go`) while faint green "Hacker Thoughts" stream in the background.
5. **Review the Report** — When the agent finishes its investigation, it types out an **Executive Summary** with an overall risk rating and a hypothetical attack chain.

> **Easter Egg:** Try typing `do a barrel roll` in the target path input. 🙃

---

## ⚙️ Configuration

Argus is configured entirely through environment variables — no config files needed.

| Variable | Default | Description |
|---|---|---|
| `ARGUS_MODEL` | `minimax-m2.5:cloud` | Ollama model to use for both Recon and Exploit roles. |
| `ARGUS_LISTEN_ADDR` | `:8080` | Host and port for the Go HTTP server. |
| `OLLAMA_HOST` | `http://localhost:11434` | Base URL for the Ollama API instance. |

---

## 🔐 The Junk Shield Sandbox

Argus operates inside a strict filesystem sandbox to prevent runaway reads and LLM context flooding.

**Blacklisted directories** (automatically skipped during traversal):
`.git`, `node_modules`, `venv`, `.venv`, `env`, `__pycache__`, `dist`, `build`, `vendor`, `.idea`, `.vscode`, `coverage`

**Blacklisted file extensions** (binary & media files):
`.exe`, `.dll`, `.so`, `.png`, `.jpg`, `.pdf`, `.zip`, `.tar`, `.gz`, `.mp4`, `.mp3`, and more.

**Hard limits:**
- **500 KB** maximum per file read.
- **50 matches** maximum per `grep_search` call.
- Path traversal (`../`) and symlink escapes are blocked at the OS level.

---

## 🗺️ Roadmap

- [ ] Automated Git Cloning — scan repositories directly from URLs.
- [ ] Multi-agent Workflows — Recon passes findings off to the Exploit agent.
- [ ] PDF & HTML Report Export.
- [ ] Granular finding severity scoring.

---

## 📖 Documentation

For a complete, in-depth reference covering architecture, API endpoints, agent internals, and the LLM provider interface, visit the community wiki generated by Devin AI:

**👉 [deepwiki.com/MeryylleA/argus-beta](https://deepwiki.com/MeryylleA/argus-beta)**

---

## 📄 License

This project is open-source and available under the [MIT License](LICENSE).

---

<div align="center">
  Built with ❤️ by <a href="https://github.com/MeryylleA">MeryylleA</a>
</div>
