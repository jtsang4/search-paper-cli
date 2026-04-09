---
name: search-paper
description: Use when the user wants to search, inspect, download, or read academic papers through search-paper-cli. This skill checks whether the CLI is available, installs the latest version automatically when missing, and runs the CLI with SEARCH_PAPER_ENV_FILE pointing at this skill directory's .env file.
license: MIT
compatibility: Requires POSIX sh. Auto-install prefers Go; Linux amd64 can also install from GitHub release artifacts with curl or wget and tar.
metadata:
  repository: github.com/jtsang4/search-paper-cli
---

# Search Paper

Use this skill instead of calling `search-paper-cli` directly.

Keep the bootstrap logic in the bundled scripts, and use `references/USAGE.md` for command usage details.

## Available scripts

- `scripts/ensure-search-paper-cli.sh` — resolves the CLI path and auto-installs the latest version when the command is missing.
- `scripts/run-search-paper-cli.sh` — validates required environment configuration and runs the CLI with `SEARCH_PAPER_ENV_FILE` bound to this skill directory's `.env` file.

## Workflow

1. If you need command examples or subcommand guidance, read `references/USAGE.md`.
2. Run the CLI through this wrapper:

   ```bash
   sh scripts/run-search-paper-cli.sh <command> [args...]
   ```

3. Do not rely on agent-specific plugin environment variables. The wrapper derives the skill root from its own script location, which works for project installs, user installs, and other Agent Skills compatible clients.
4. If `search-paper-cli` is missing, the wrapper auto-installs the latest version. It prefers `go install github.com/jtsang4/search-paper-cli/cmd/search-paper-cli@latest` and falls back to the latest Linux amd64 release artifact when possible.
5. Create `.env` next to `.env.example` in this skill directory when local configuration is needed.
6. Required variable: `SEARCH_PAPER_UNPAYWALL_EMAIL`.
7. This skill only recognizes `SEARCH_PAPER_`-prefixed environment variables. Do not infer non-prefixed or fuzzy-matched variable names. If configuration is missing, explicitly tell the user to add the exact prefixed variable, for example `SEARCH_PAPER_UNPAYWALL_EMAIL=you@example.com`.
8. If the wrapper reports missing required environment configuration, stop and tell the user to create or update the skill-local `.env`, then rerun the same command.
9. All other `SEARCH_PAPER_*` variables from `.env.example` are optional.
10. `search --year` is a hard constraint. Pass it only in `YYYY` or `YYYY-YYYY` form and expect the CLI to enforce the year locally on final results, not only as a best-effort upstream hint.
11. If some sources fail but others succeed, treat the result as degraded mode rather than full success. Inspect the returned failed-source fields and errors instead of assuming every requested source completed.
12. Prefer the CLI's default JSON output unless the user explicitly asks for text output.
