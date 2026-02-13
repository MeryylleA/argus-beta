# Changelog
All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses Semantic Versioning.

## [0.1.0] - 2026-02-13
### Added
- Tool executor with bounded execution and timeout controls.
- CLI command surface for:
  - `search_code`
  - `view_lines`
  - `find_files`
  - `directory_tree`
  - `git_log`
- Sandbox path validation with canonicalization using symlink-aware resolution.
- Audit logger for tool actions and result status.

### Security
- Path traversal prevention through canonical path validation.
- Symlink escape detection to block root-boundary bypass.
- Subprocess isolation controls for external tool execution.
