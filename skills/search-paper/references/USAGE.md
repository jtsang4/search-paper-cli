# Usage

Use this reference when the skill is active and you need to understand which subcommand to use, what each subcommand expects, and common invocation patterns.

Always invoke through the wrapper:

```bash
sh scripts/run-search-paper-cli.sh <command> [args...]
```

## Command summary

- `sources` — inspect available sources, capability states, and whether credentials have enabled gated providers
- `search` — search one or more sources and return normalized paper results
- `download` — fetch paper full text when a source or fallback path supports downloading
- `read` — fetch and extract readable paper content when supported
- `version` — print the CLI version

## Inspect sources

Use `sources` when you need to know what providers are available before searching or retrieving.

```bash
sh scripts/run-search-paper-cli.sh sources
sh scripts/run-search-paper-cli.sh sources --format text
sh scripts/run-search-paper-cli.sh sources --source arxiv,semantic
```

Typical flags:

- `--source <csv>` filters to one or more source ids
- `--format json|text` selects machine-readable or human-readable output

## Search papers

Use `search` to query one or more providers and get normalized paper metadata.

```bash
sh scripts/run-search-paper-cli.sh search --source arxiv --limit 5 "graph neural networks"
sh scripts/run-search-paper-cli.sh search --source semantic --year 2024 --limit 10 "retrieval augmented generation"
```

Typical flags:

- `--source <csv>` chooses one or more sources
- `--limit <n>` caps requested results per source
- `--year <value>` is forwarded to Semantic Scholar searches
- final positional argument is the search query

Search output includes normalized papers plus metadata like requested sources, used sources, per-source result counts, and errors.

## Download a paper

Use `download` when you already have a paper object or enough metadata to retrieve a file.

```bash
sh scripts/run-search-paper-cli.sh download --source arxiv --paper-json '{"paper_id":"1234.5678v1","title":"Example","source":"arxiv","pdf_url":"https://arxiv.org/pdf/1234.5678v1.pdf"}'
```

Typical inputs:

- `--source <id>`
- `--paper-json '<json>'`
- or source-specific identifiers such as paper id / DOI / URL depending on the workflow
- optional save-directory arguments when you need control over output location

## Read a paper

Use `read` when you want extracted paper content instead of a saved file.

```bash
sh scripts/run-search-paper-cli.sh read --source arxiv --paper-json '{"paper_id":"1234.5678v1","title":"Example","source":"arxiv","pdf_url":"https://arxiv.org/pdf/1234.5678v1.pdf"}'
```

Typical inputs mirror `download`, but the result is structured read output rather than a saved artifact path.

## Version

```bash
sh scripts/run-search-paper-cli.sh version
```

## Notes

- Prefer the CLI's default JSON output unless the user explicitly asks for text
- If the wrapper reports missing configuration, create the skill-local `.env` first
- If the CLI is missing, the wrapper will install the latest version automatically when possible
