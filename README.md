# search-paper-cli

`search-paper-cli` is an agent-friendly Go CLI replacement for `paper-search-mcp`. It provides consistent machine-readable search, download, and read flows across academic paper sources, supports OA-first fallback retrieval, and is designed to work both from the repository and as a packaged standalone Linux binary.

## Key features

- Go CLI with JSON-first output and optional `--format text`
- Main commands: `sources`, `search`, `download`, `read`, `version`
- Normalized paper schema across sources: `paper_id`, `title`, `authors`, `abstract`, `doi`, `published_date`, `pdf_url`, `url`, `source`
- Built-in source registry with capability reporting and gated-source handling
- Native retrieval plus optional OA-first fallback chain and optional Sci-Hub fallback
- `.env` discovery that works in-repo and for packaged binaries outside the repo

## Installation / build

Requires Go `1.26`.

```bash
go mod tidy
go build ./...
go build -o dist/search-paper-cli ./cmd/search-paper-cli
```

Install the CLI directly:

```bash
go install github.com/jtsang4/search-paper-cli/cmd/search-paper-cli@latest
```

## Usage examples

Show help and version:

```bash
search-paper-cli --help
search-paper-cli version
```

Inspect source capabilities:

```bash
search-paper-cli sources
search-paper-cli sources --format text
search-paper-cli sources --source arxiv,semantic,pmc
```

Search papers:

```bash
search-paper-cli search "graph neural networks"
search-paper-cli search --source semantic,crossref --limit 5 "multimodal agents"
search-paper-cli search --source semantic --year 2024-2025 "agentic retrieval"
```

Download or read a known paper record:

```bash
search-paper-cli download --source arxiv --paper-json '{"paper_id":"1234.5678","title":"Example","pdf_url":"https://example.org/paper.pdf","source":"arxiv"}'
search-paper-cli read --source pmc --paper-json '{"paper_id":"PMC123","title":"Example","pdf_url":"https://example.org/paper.pdf","source":"pmc"}'
```

Use OA-first fallback retrieval:

```bash
search-paper-cli download --fallback --save-dir ./downloads --paper-json '{"paper_id":"1234.5678","title":"Example","doi":"10.1000/example","source":"arxiv"}'
search-paper-cli download --fallback --allow-scihub --scihub-base-url https://sci-hub.se --paper-json '{"paper_id":"1234.5678","title":"Example","doi":"10.1000/example","source":"arxiv"}'
```

Supported source ids:

```text
acm, arxiv, base, biorxiv, citeseerx, core, crossref, dblp, doaj, europepmc,
google-scholar, hal, iacr, ieee, medrxiv, openalex, openaire, pmc, pubmed,
semantic, scihub, ssrn, unpaywall, zenodo
```

## Environment variables

Common runtime variables:

- `SEARCH_PAPER_UNPAYWALL_EMAIL` (`UNPAYWALL_EMAIL` legacy alias): enables Unpaywall DOI/OA lookup
- `SEARCH_PAPER_CORE_API_KEY` (`CORE_API_KEY` legacy alias): recommended for CORE reliability/rate limits
- `SEARCH_PAPER_SEMANTIC_SCHOLAR_API_KEY`
- `SEARCH_PAPER_GOOGLE_SCHOLAR_PROXY_URL`
- `SEARCH_PAPER_DOAJ_API_KEY`
- `SEARCH_PAPER_ZENODO_ACCESS_TOKEN`
- `SEARCH_PAPER_IEEE_API_KEY`
- `SEARCH_PAPER_ACM_API_KEY`

Optional endpoint overrides for deterministic tests and artifact validation:

- `SEARCH_PAPER_ARXIV_BASE_URL`
- `SEARCH_PAPER_OPENAIRE_BASE_URL`
- `SEARCH_PAPER_OPENAIRE_LEGACY_BASE_URL`
- `SEARCH_PAPER_CORE_BASE_URL`
- `SEARCH_PAPER_EUROPEPMC_BASE_URL`
- `SEARCH_PAPER_PMC_SEARCH_URL`
- `SEARCH_PAPER_PMC_SUMMARY_URL`
- `SEARCH_PAPER_UNPAYWALL_BASE_URL`

`.env` loading order:

1. `SEARCH_PAPER_ENV_FILE`
2. `./.env` in the current working directory
3. Repository-root `.env` when running inside the source tree

Prefixed `SEARCH_PAPER_*` variables override legacy aliases, and an explicitly empty prefixed value blocks fallback.

## Testing / packaging

Project validation commands:

```bash
GOMAXPROCS=8 go test -run '^$' -p 8 ./...
test -z "$(gofmt -l .)"
GOMAXPROCS=8 go test -count=1 -p 8 ./...
GOMAXPROCS=8 go build ./...
```

Build release artifacts:

```bash
./scripts/package-release.sh
```

Release artifact paths:

- `dist/search-paper-cli_linux_amd64/search-paper-cli`
- `dist/search-paper-cli_linux_amd64.tar.gz`

## License

This project is licensed under the [MIT License](./LICENSE).

## Notes on output contract and optional sources

- Default output is JSON; use `--format text` for a human-readable view
- Success responses use `{"status":"ok", ...}` and structured failures use `{"status":"error", ...}`
- `search` returns normalized papers plus `requested_sources`, `used_sources`, `invalid_sources`, `source_results`, and per-source `errors`
- `download` and `read` return operation state, result path/content, and fallback `attempts`
- Use `sources` to inspect whether a source is `supported`, `record_dependent`, `informational`, `unsupported`, or `gated`
- Some sources are optional or gated by credentials (`ieee`, `acm`), and some retrieval paths are metadata-only or record-dependent rather than direct-download capable
