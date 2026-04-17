---
name: search-paper
description: Use when the user wants to search, inspect, or retrieve academic papers through search-paper-cli. This skill keeps the workflow on direct `search-paper-cli ...` commands, can auto-install the CLI when missing, and relies on process env plus `~/.config/search-paper-cli/config.yaml` / `config.yml` for runtime configuration.
license: MIT
compatibility: Requires POSIX sh. Auto-install prefers Go; Linux amd64 can also install from GitHub release artifacts with curl or wget and tar.
metadata:
  repository: github.com/jtsang4/search-paper-cli
---

# Search Paper

Use this skill when you need to run `search-paper-cli` directly.

Keep command usage in `references/USAGE.md`. The bundled helper script is install/locate-only and does not inject runtime configuration.

## Available scripts

- `scripts/ensure-search-paper-cli.sh` — resolves the CLI path and auto-installs the latest version when the command is missing.

## Workflow

1. If you need command examples or subcommand guidance, read `references/USAGE.md`.
2. Run the CLI directly:

   ```bash
   search-paper-cli <command> [args...]
   ```

   Prefer `get --as pdf` for file retrieval and `get --as text` for extracted-content retrieval. Legacy `download` and `read` aliases still work, but prefer `get` so the retrieval target stays explicit for agents.

3. If `search-paper-cli` is missing, run `sh scripts/ensure-search-paper-cli.sh` to locate or install it. After that, invoke the CLI directly from your PATH, or prepend the returned binary directory to `PATH` for the current shell.
4. Runtime configuration comes only from exact `SEARCH_PAPER_*` environment variables plus `~/.config/search-paper-cli/config.yaml` with `config.yml` fallback.
5. Process environment wins per key over the global config file. Explicitly empty env values still count as overrides and block fallback for that key.
6. Required variable / config key for Unpaywall-backed behavior: `SEARCH_PAPER_UNPAYWALL_EMAIL` / `unpaywall_email`.
7. This skill only recognizes exact `SEARCH_PAPER_`-prefixed environment variables. Do not infer non-prefixed or fuzzy-matched variable names.
8. Do not use `SEARCH_PAPER_ENV_FILE`, cwd `.env`, repository-root `.env`, or skill-local `.env` files as runtime inputs. They are ignored by the direct CLI workflow.
9. `search --year` is a hard constraint. Pass it only in `YYYY` or `YYYY-YYYY` form and expect the CLI to enforce the year locally on final results, not only as a best-effort upstream hint.
10. If some sources fail but others succeed, treat the result as degraded mode rather than full success. Inspect the returned failed-source fields and errors instead of assuming every requested source completed.
11. Prefer the CLI's default JSON output unless the user explicitly asks for text output.
12. Avoid presenting agent callers with ambiguous retrieval choices. Use `search` for discovery, then `get --as pdf|text` for retrieval. Only use legacy `download` or `read` when compatibility with older callers is required.
