# Argus AI Security Scanner 👁️

Argus is an autonomous, open-source AI security intelligence agent powered by Go and Svelte 5. It acts as a local security researcher that maps, sniffs, and deeply analyzes your codebases for vulnerabilities, hardcoded secrets, misconfigurations, and potentially exploitable flows.

By leveraging local LLMs (like Ollama) or cloud providers, Argus operates completely autonomously within a restricted filesystem sandbox ("Junk Shield"). It chains tool calls (`read_file`, `list_directory`, `search_code`, `grep_search`, `git_blame`, etc.) dynamically based on its ongoing intelligence gathering, eventually providing an executive summary with a hypothetical attack chain.

---

## ⚡ Features

* **Autonomous Recon & Exploit Modes:** Instruct the agent to either map your architecture (Recon) or trace user inputs to dangerous sinks (Exploit).
* **Cyberpunk Reactive UI:** A highly polished, chat-centric Svelte 5 frontend with real-time SSE streaming. Watch the agent's raw inner "thoughts" and tools slide through the terminal background in real-time.
* **Junk Shield Sandbox:** Argus is prevented from hallucinating or context-window crashing by strictly blacklisting directories (`node_modules`, `.git`, etc.) and enforcing a 500KB cap on source code files.
* **Extensible Go Backend:** Designed with clean, stateless HTTP API architecture, Server-Sent Events (SSE), and modular tool registries.
* **Local-First LLMs:** Works out of the box with `ollama`. Simply switch the `ARGUS_MODEL` and keep your code private.

---

## 🚀 Getting Started

To run Argus, you need to run both the Go Backend and the SvelteKit Frontend.

### Prerequisites

* [Go 1.23+](https://go.dev/doc/install)
* [Node.js 20+ & npm](https://nodejs.org/en)
* (Optional but recommended) [Ollama](https://ollama.com/) running locally for private scans.

### 1. Start the Go Backend

The backend serves the API and the SSE stream on `http://localhost:8080`.

```bash
# Clone the repo
git clone https://github.com/MeryylleA/argus-beta.git
cd argus-beta

# Install Go dependencies
go mod tidy

ARGUS_MODEL="ollama:minimax-m2.5:cloud" go run cmd/argus/main.go
```

*(Note: Adjust the `ARGUS_MODEL` environment variable according to the models you have pulled in Ollama or the cloud provider logic you have implemented).*

### 2. Start the Svelte Frontend

The frontend gives you the gorgeous Cyberpunk terminal UI to kick off scans and view real-time findings.

```bash
# Open a new terminal tab
cd argus-beta/frontend

# Install Node dependencies
npm install

# Start the SvelteKit development server
npm run dev
```

The frontend will run on `http://localhost:5173`. Open your browser to that address.

---

## 🛠️ Usage

1. **Enter a Target Path** 
   Open `http://localhost:5173`. In the central input box, type the **absolute path** to the repository or folder you want to scan on your machine (e.g., `/home/user/projects/my-app`).
2. **Start Scan** 
   Hit the button. The backend will spawn a session and immediately begin Server-Sent Events (SSE).
3. **Watch the Magic** 
   You will see the Svelte terminal update with the agent's actions (`Decrypting source code: src/main.go`) while faint, green "Hacker Thoughts" stream rapidly in the background. Badges will appear instantly if vulnerabilities are discovered.
4. **View the Report**
   When the AI finishes exhausting all paths of investigation, it will type out an **Executive Summary** detailing the overall risk and a hypothetical attack chain.

*(P.S. Try typing `do a barrel roll` in the target path input.)*

---

## 📂 Architecture

* **`cmd/argus/main.go`**: The entry point. Initializes the SQLite memory DB, loads config, and starts the API server.
* **`internal/api/`**: Handlers and the `Server-Sent Events (SSE)` hub that bridges the agent's loop to the frontend.
* **`internal/agent/`**: The core AI logic.
  * `runner.go`: The `executeTool` loop and system prompts.
  * `sandbox.go`: The "Junk Shield", directory traversing, file reading limits, and the `GrepSearch` high-speed radar.
  * `tools/`: Implementations of Git Blame, Find Secrets, etc.
* **`internal/llm/`**: The abstracted provider layer (currently defaults to processing Ollama NDJSON streams).
* **`frontend/`**: The Svelte 5 application containing the store (`agent.svelte.ts`) and the UI components (`+page.svelte`, `FindingCard.svelte`).

---

## 🛡️ Roadmap

* [ ] Add automated Git Cloning to target repositories directly from URLs.
* [ ] Implement more granular multi-agent workflows (Recon passes off to Exploit).
* [ ] Add PDF and HTML report exportation.

---

## License

This project is open-source and available under the MIT License.
