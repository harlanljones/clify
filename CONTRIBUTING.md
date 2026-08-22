# Contributing to clify

Thank you for your interest in contributing to `clify` and `cliamp-clify`!

This repository is organized as a polyglot [Turborepo](https://turbo.build/repo) monorepo containing:
- **[`cliamp-clify`](cliamp-clify/)**: Retro terminal music player fork (Go 1.26+).
- **[`packages/clify`](packages/clify/)**: Complementary CLI & Metric-Driven Agent-Driven Development (ADD) framework (Python 3.10+).

Please read this guide to set up your local development environment and understand our development workflow.

---

## Code of Conduct

All contributors are expected to uphold our [Code of Conduct](CODE_OF_CONDUCT.md). Please report unacceptable behavior following the reporting guidelines in that document.

---

## Prerequisites

To build and test the entire monorepo locally, ensure you have installed:
- **Node.js** ≥ 18 and **[pnpm](https://pnpm.io/)** ≥ 9
- **Go** ≥ 1.26
- **Python** ≥ 3.10 (along with `pip`, `uv`, or a virtual environment)
- **ALSA development headers** (Linux builds of `cliamp-clify`): e.g. `libasound2-dev` on Debian/Ubuntu, `alsa-lib-devel` on Fedora, `alsa-lib` on Arch.

---

## Getting Started

1. **Fork and clone the repository:**
   ```bash
   git clone --recursive https://github.com/harlanljones/clify.git

   cd clify
   ```

2. **Install root dependencies:**
   ```bash
   pnpm install
   ```

3. **Set up Python development environment:**
   ```bash
   # Using a virtual environment
   python3 -m venv .venv
   source .venv/bin/activate
   pip install -e "packages/clify[test]"
   ```

4. **Verify the complete test suite via Turborepo:**
   ```bash
   pnpm test
   ```

---

## Monorepo Workflow (Turborepo)

Root `pnpm` commands orchestrate both Go and Python workspaces with full artifact and test caching:

| Command | Action |
|---|---|
| `pnpm build` | Compiles the `cliamp-clify` Go binary and builds Python packages |
| `pnpm test` | Runs Go test suites (`go test ./...`) and Pytest in parallel |
| `pnpm check` | Runs linters, formatting checks, and non-integration tests |
| `pnpm lint` | Runs `go vet` and Python compile/lint checks |
| `pnpm clean` | Cleans build artifacts and caches |

---

## Working on `cliamp-clify` (Go)

The Go music player codebase resides in `cliamp-clify/`.

```bash
cd cliamp-clify

# Run tests
go test ./...

# Format, vet, and test
make check

# Build binary
make build

# Install to ~/.local/bin
make install
```

When modifying TUI layouts or IPC methods, ensure you maintain backwards compatibility with the `cliamp.history.unified/1` contract and preserve stock cliamp configuration compatibility (`~/.config/cliamp/`).

---

## Working on `clify` (Python)

The Python CLI and ADD framework code resides in `packages/clify/`.

```bash
cd packages/clify

# Run unit and contract tests
pytest

# Run tests excluding live daemon integrations
pytest -m "not integration"

# Run recorded Spotify API contract tests
pytest tests/test_spotify_client.py tests/test_spotify_contract.py
```

### Metric-Driven ADD & TDD Lifecycle

All agent additions and modifications must adhere to the three-stage TDD lifecycle defined in [AGENTS.md](AGENTS.md):
1. **Red Stage:** Write boundary enforcement, schema conformance, and SLA metric assertions first.
2. **Green Stage:** Implement minimal reasoning logic and inject mocked system tools.
3. **Refactor Stage:** Optimize prompts, context windows, and performance while ensuring SLA compliance (Latency $\le 2.5s$, Cost $\le \$0.02$, Confidence $\ge 0.90$).

---

## Submitting Pull Requests

1. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/my-enhancement
   ```
2. Make changes following clean coding and testing standards.
3. Ensure all Turborepo checks and tests pass:
   ```bash
   pnpm check
   pnpm test
   ```
4. Commit your changes with clear, descriptive commit messages following the Conventional Commits specification (e.g. `feat:`, `fix:`, `docs:`, `test:`).
5. Push to your fork and open a Pull Request against the `main` branch.
6. Check that all GitHub Actions CI checks turn green on your PR.

---

## Reporting Issues & Security

- **Bugs and Feature Requests:** Please open an issue using the appropriate template in the [Issues](https://github.com/harlanljones/clify/issues) tab.

- **Security Vulnerabilities:** Please review our [Security Policy](SECURITY.md) for responsible disclosure instructions.
