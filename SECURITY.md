# Security Policy

## Supported Versions

We actively provide security patches for the following versions:

| Product | Supported Versions |
|---|---|
| `cliamp-clify` | `v1.63.2-clify.*` |
| `clify` CLI & SDK | `1.6.*` |

---

## Security Guarantees & Credentials Handling

Both `cliamp-clify` and `clify` are designed with defensive credential handling practices:

1. **PKCE OAuth & Secretless Authentication:**
   - Spotify authentication uses OAuth 2.0 Authorization Code with PKCE (Proof Key for Code Exchange).
   - No Client Secrets are required, transmitted, or persisted.
2. **Strict File Permissions:**
   - OAuth tokens are saved to `~/.config/clify/spotify.json` with strict mode `0600` (readable and writable only by the owning user).
3. **Telemetry & Log Redaction:**
   - All access tokens, refresh tokens, and Authorization headers are automatically redacted from error traces, CLI outputs, and monitoring telemetry payloads.
4. **Deterministic Scope Guardrails:**
   - The Metric-Driven ADD framework blocks sensitive actions (e.g. `user.billing`) deterministically at the scope gate before any tool or network call executes.

---

## Reporting a Vulnerability

If you discover a potential security vulnerability in this project:

1. **Do not disclose it publicly** in public GitHub issues, discussions, or pull requests.
2. Send a report directly to the maintainers or use [GitHub Private Vulnerability Reporting](https://github.com/harlan/clify/security/advisories/new).
3. Include detailed reproduction steps, the affected component (`cliamp-clify` or `clify`), and sample payloads if applicable.
4. Maintainers will acknowledge your report within 48 hours and coordinate a fix and release.
