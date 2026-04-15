# AGENTS

## Purpose

This repository provides `search-paper-cli`, an agent-friendly Go CLI for searching academic papers and retrieving paper content in machine-readable formats.

## Recommended agent entrypoint

Prefer using the bundled skill instead of invoking the CLI directly.

- Skill directory: `skills/search-paper`
- Skill manifest: `skills/search-paper/SKILL.md`
- Wrapper command:

```bash
sh skills/search-paper/scripts/run-search-paper-cli.sh <command> [args...]
```

The wrapper binds `SEARCH_PAPER_ENV_FILE` to the skill-local `.env`, validates required configuration, and auto-installs the CLI when missing.

## Agent usage guidance

- Prefer JSON output unless the user explicitly asks for text output.
- Prefer `get --as pdf` for PDF retrieval.
- Prefer `get --as text` for extracted-content retrieval.
- Legacy `download` and `read` commands still exist, but `get --as ...` is the preferred agent-facing interface.
- Treat `search --year` as a hard constraint and pass it only as `YYYY` or `YYYY-YYYY`.
- If some requested sources fail while others succeed, treat the result as degraded mode and inspect the returned failed-source details.

## Environment configuration

For agent-driven usage, keep `.env` next to the skill files:

- `skills/search-paper/.env.example`
- `skills/search-paper/.env`

Required variable:

- `SEARCH_PAPER_UNPAYWALL_EMAIL`

Only `SEARCH_PAPER_*` variables are recognized.

CLI `.env` discovery order:

1. `SEARCH_PAPER_ENV_FILE`
2. `./.env` in the current working directory
3. Repository-root `.env` when running inside the source tree

## Installation

Install from source with Go:

```bash
go install github.com/jtsang4/search-paper-cli/cmd/search-paper-cli@latest
```

The skill wrapper can also auto-install the CLI:

1. `SEARCH_PAPER_CLI_BIN` when it points to an executable
2. `search-paper-cli` from `PATH`
3. `skills/search-paper/bin/search-paper-cli`
4. `go install github.com/jtsang4/search-paper-cli/cmd/search-paper-cli@latest`
5. Latest Linux amd64 release artifact download

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
- `internal/config` — environment loading and config resolution
- `internal/connectors` — source integrations
- `internal/paper` — normalized paper model helpers
- `internal/sources` — source registry and capability metadata
- `internal/release` — release and skill packaging validation
- `skills/search-paper` — Agent Skill wrapper, examples, and helper scripts
- `scripts/package-release.sh` — release packaging script
