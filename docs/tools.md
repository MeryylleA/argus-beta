# Argus Tool Reference
This reference describes the Phase 1 tool surface: behavior, parameters, examples, and built-in security constraints.

## 1) `search_code`
Purpose: search repository content using regex patterns.

Parameters:
- `pattern` (string): regex to search
- `path` (string): repository-relative or absolute path inside sandbox root
- `max_results` (int, optional): max matches returned

Example input:
```bash
argus tool search_code "(auth|jwt|token|password)" ./internal 25
```

Example output:
```text
internal/auth/jwt.go:42: token := strings.TrimSpace(header)
internal/http/middleware.go:13: if !validateAuth(req) { ... }
```

Security constraints:
- Path must remain within sandbox root.
- Uses `rg --no-follow` to avoid symlink traversal surprises.
- Read-only search only.

## 2) `view_lines`
Purpose: show a bounded line range from a file.

Parameters:
- `file` (string)
- `start_line` (int)
- `end_line` (int, max bounded by tool policy)

Example input:
```bash
argus tool view_lines internal/core/tools/sandbox.go 1 20
```

Example output format:
```text
   1 | package main
   2 |
   3 | import "fmt"
```

Security constraints:
- Canonical path validation before read.
- Hard max range (Phase 1: 100 lines per request).

## 3) `find_files`
Purpose: locate files by name/pattern and extension.

Parameters:
- `path` (string)
- `pattern` (string, optional)
- `extension` (string, optional)

Example input:
```bash
argus tool find_files . "*.go" go
```

Example output:
```text
internal/core/tools/sandbox.go
internal/core/tools/executor.go
cmd/argus/main.go
```

`fd` vs `find` behavior:
- `fd` is preferred for speed and cleaner defaults.
- If `fd` is unavailable, Argus falls back to `find`.
- Both execution paths stay read-only and sandbox-restricted.

Security constraints:
- Search root is validated and bounded.
- No file writes or execution side effects.

## 4) `directory_tree`
Purpose: render a repository tree for orientation.

Parameters:
- `path` (string)
- `depth` (int, optional)

Example input:
```bash
argus tool directory_tree . 2
```

Example output:
```text
.
├── cmd
│   └── argus
├── internal
│   └── core
└── go.mod
```

Skipped directories:
- Large/noisy directories (for example VCS and common dependency caches) may be skipped by policy.
- Depth limiting prevents oversized outputs.

Security constraints:
- Pure Go traversal under sandbox path checks.
- Symlink resolution is validated against root boundary.

## 5) `git_log`
Purpose: inspect commit history for context and blame-free chronology.

Parameters:
- `path` (string)
- `count` (int, optional)

Example input:
```bash
argus tool git_log . 10
```

Example output:
```text
abc1234 fix: tighten sandbox path checks
def5678 feat: add directory_tree tool
```

Security constraints:
- Uses `GIT_CEILING_DIRECTORIES` to prevent repository traversal above allowed boundaries.
- Read-only invocation (`git log`) only.
