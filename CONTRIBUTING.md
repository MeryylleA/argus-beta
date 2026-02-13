# Contributing to Argus
Thanks for contributing to Argus. This project targets bug bounty researchers and security engineers, so correctness and sandbox safety take priority over speed.

## Development Environment Setup
### Requirements
- Go `1.22+`
- `git`
- `rg` (ripgrep)
- `fd` (or `find` fallback)
- `golangci-lint`
- `govulncheck`

### Setup
```bash
git clone https://github.com/MeryylleA/argus-beta.git
cd argus-beta
go mod download
make install-deps
```

## Code Standards
All contributions must pass:
- `go fmt ./...`
- `go vet ./...`
- `staticcheck ./...`
- `golangci-lint run`

Keep changes focused, deterministic, and auditable. Prefer explicit error handling and avoid side effects in tool execution paths.

## Testing Requirements
- `go test ./... -race` must pass.
- All sandbox and path validation tests must pass.
- New tools must include sandbox-focused tests.
- Security-sensitive code changes require regression tests for traversal/symlink scenarios.

## Pull Request Process
Use conventional commit types in PR titles and commits:
- `feat:` new functionality
- `fix:` bug fixes
- `sec:` security fixes/hardening
- `docs:` documentation-only changes
- `test:` test-only updates

PR expectations:
1. Keep PRs narrow in scope.
2. Include tests for behavior changes.
3. Document security impact when touching executor/sandbox/tool code.
4. Link issues where applicable.

## Security-Specific Contribution Guidelines
- Do not include tests that execute or weaponize real exploits.
- Do include defensive tests that simulate malicious input safely.
- Never bypass read-only guarantees.
- Never weaken path canonicalization or symlink protections.
- Keep tool subprocess environments minimal and controlled.

## Branch Strategy
- `main`: stable branch
- `develop`: integration branch for upcoming releases
- `feature/*`: new features
- `fix/*`: bug fixes
- `sec/*`: security fixes and hardening

## Questions
Open a discussion or a non-security issue for design and roadmap questions. For vulnerabilities, use private disclosure only (see `SECURITY.md`).
