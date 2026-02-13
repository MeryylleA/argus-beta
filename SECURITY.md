# Security Policy
Argus is a security tool. Security defects in Argus can mislead research or weaken user safety, so they are treated as high priority.

## Supported Versions
| Version | Supported |
| --- | --- |
| 0.1.x | ✅ Yes |
| < 0.1.0 | ❌ No |

## Reporting a Vulnerability
Use **GitHub Security Advisories** for private disclosure:
1. Open a private advisory in the repository security tab.
2. Include impact, reproduction steps, and affected versions.
3. Include whether the issue affects sandbox boundaries, path checks, or tool isolation.

Do **not** open public GitHub issues for vulnerabilities.

## What Qualifies as a Security Issue in Argus
Examples:
- Sandbox bypass
- Path traversal or path canonicalization bypass
- Symlink escape bypass
- Unauthorized write/execute/network behavior from tools
- Logging leaks of sensitive data
- Unsafe subprocess environment handling

## Response Timeline
- Initial acknowledgment: within **48 hours**
- Triage and severity classification: as soon as reproducible details exist
- Critical vulnerability patch target: within **7 days**
- Coordinated disclosure: after patch release and maintainer confirmation

## Out of Scope
Vulnerabilities found in repositories analyzed by Argus are out of scope for this policy. Discovering those vulnerabilities is Argus's intended purpose.

## Priority Note
Because Argus enforces defensive boundaries for hostile codebases, any issue affecting sandbox guarantees is treated as **critical priority**.
