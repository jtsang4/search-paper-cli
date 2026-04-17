# AGENTS

## Purpose

This repository provides `search-paper-cli`, an agent-friendly Go CLI for searching academic papers and retrieving paper content in machine-readable formats.

## Recommended agent entrypoint

Prefer invoking `search-paper-cli` directly.

- Primary command form:

```bash
search-paper-cli <command> [args...]
```

- Skill directory: `skills/search-paper`
- Skill manifest: `skills/search-paper/SKILL.md`

The skill should describe the same direct CLI workflow rather than a wrapper-managed runtime path.

## Agent usage guidance

- Prefer JSON output unless the user explicitly asks for text output.
- Prefer `get --as pdf` for PDF retrieval.
- Prefer `get --as text` for extracted-content retrieval.
- Legacy `download` and `read` commands still exist, but `get --as ...` is the preferred agent-facing interface.
- Treat `search --year` as a hard constraint and pass it only as `YYYY` or `YYYY-YYYY`.
- If some requested sources fail while others succeed, treat the result as degraded mode and inspect the returned failed-source details.

## Environment configuration

The CLI reads runtime configuration from:

1. Process environment variables with exact `SEARCH_PAPER_*` names
2. `~/.config/search-paper-cli/config.yaml`
3. `~/.config/search-paper-cli/config.yml` when `config.yaml` is absent

Per-key merge semantics apply: environment variables win for the same key, and missing env keys fall back to the global config file. Explicitly empty env values still count as overrides and block fallback for that key.

Supported global-config keys are lowercase snake_case forms of the existing settings, including:

- `unpaywall_email`
- `core_api_key`
- `semantic_scholar_api_key`
- `google_scholar_proxy_url`
- `doaj_api_key`
- `zenodo_access_token`
- `ieee_api_key`
- `acm_api_key`
- `arxiv_base_url`
- `openaire_base_url`
- `openaire_legacy_base_url`
- `core_base_url`
- `europepmc_base_url`
- `pmc_search_url`
- `pmc_summary_url`
- `unpaywall_base_url`

Only exact `SEARCH_PAPER_*` environment variables are recognized. Do not rely on `SEARCH_PAPER_ENV_FILE`, cwd `.env`, repository-root `.env`, or skill-local `.env` as runtime configuration sources.

Required variable / config key for Unpaywall-backed behavior:

- `SEARCH_PAPER_UNPAYWALL_EMAIL` / `unpaywall_email`

## Installation

Install from source with Go:

```bash
go install github.com/jtsang4/search-paper-cli/cmd/search-paper-cli@latest
```

## Build

Requires Go `1.26`.

Local build commands:

```bash
go mod tidy
go build ./...
go build -o dist/search-paper-cli ./cmd/search-paper-cli
```

## Validation

Run these before finishing code changes:

```bash
GOMAXPROCS=8 go test -run '^$' -p 8 ./...
test -z "$(gofmt -l .)"
GOMAXPROCS=8 go test -count=1 -p 8 ./...
GOMAXPROCS=8 go build ./...
```

Use temp `HOME` directories for config-loading tests so the host's real `~/.config/search-paper-cli` never affects validation.

## Release packaging

Build the Linux amd64 release artifact with:

```bash
./scripts/package-release.sh
```

Expected outputs:

- `dist/search-paper-cli_linux_amd64/search-paper-cli`
- `dist/search-paper-cli_linux_amd64.tar.gz`

## Repository layout

- `cmd/search-paper-cli` — CLI entrypoint
- `internal/cli` — Cobra command wiring and command execution
- `internal/config` — runtime config loading and config resolution
- `internal/connectors` — source integrations
- `internal/paper` — normalized paper model helpers
- `internal/sources` — source registry and capability metadata
- `internal/release` — release and skill packaging validation
- `skills/search-paper` — Agent Skill manifest, references, and helper assets
- `scripts/package-release.sh` — release packaging script
