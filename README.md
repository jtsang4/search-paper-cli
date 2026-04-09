# search-paper-cli

`search-paper-cli` is an agent-friendly Go CLI replacement for [`paper-search-mcp`](https://github.com/openags/paper-search-mcp). It provides consistent machine-readable search, download, and read flows across academic paper sources, supports OA-first fallback retrieval, and is designed to work both from the repository and as a packaged standalone Linux binary.

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

Configure variables in a `.env` file (recommended) or via shell exports:

```bash
cp .env.example .env
```

### Credential and API key requirements

| Environment variable | Provider / usage | Required? | How to obtain / notes |
| --- | --- | --- | --- |
| `SEARCH_PAPER_UNPAYWALL_EMAIL` (`UNPAYWALL_EMAIL` legacy alias) | Unpaywall DOI/OA lookup and OA-first fallback resolution | Required for Unpaywall-backed lookup/fallback | Use a valid email address; see [unpaywall.org/products/api](https://unpaywall.org/products/api) |
| `SEARCH_PAPER_CORE_API_KEY` (`CORE_API_KEY` legacy alias) | CORE | Optional, recommended | Free API key from [core.ac.uk/services/api](https://core.ac.uk/services/api) |
| `SEARCH_PAPER_SEMANTIC_SCHOLAR_API_KEY` | Semantic Scholar | Optional | Free API key from [semanticscholar.org/product/api](https://www.semanticscholar.org/product/api); improves rate limits |
| `SEARCH_PAPER_GOOGLE_SCHOLAR_PROXY_URL` | Google Scholar proxy | Optional | Provide your own HTTP/HTTPS proxy URL if Google Scholar is rate-limited or bot-protected |
| `SEARCH_PAPER_DOAJ_API_KEY` | DOAJ | Optional | Free API key from [doaj.org/apply-for-api-key](https://doaj.org/apply-for-api-key/) |
| `SEARCH_PAPER_ZENODO_ACCESS_TOKEN` | Zenodo | Optional | Create a personal access token at [zenodo.org/account/settings/applications](https://zenodo.org/account/settings/applications/); useful for authenticated access such as private records |
| `SEARCH_PAPER_IEEE_API_KEY` | IEEE Xplore | Optional overall, required to enable the `ieee` source | Available from [developer.ieee.org](https://developer.ieee.org/) |
| `SEARCH_PAPER_ACM_API_KEY` | ACM Digital Library | Optional overall, required to enable the `acm` source | See [libraries.acm.org/digital-library/acm-open](https://libraries.acm.org/digital-library/acm-open) |

### Optional endpoint overrides

These variables are all optional and are mainly intended for deterministic tests, local mocks, or custom deployments. Most users should leave them unset.

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
