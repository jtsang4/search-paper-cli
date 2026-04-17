# Architecture

How the system works at a high level.

**What belongs here:** components, boundaries, data flow, invariants, release shape.
**What does NOT belong here:** low-level implementation notes or one-off bug lists.

---

## System overview

`search-paper-cli` is a single Go CLI that exposes paper-source discovery, search, and retrieval through a JSON-first command surface.

Primary command families:

- `sources`
- `search`
- `get`
- legacy `download` / `read`
- `version`

The product is agent-friendly and should be runnable directly as `search-paper-cli ...` both from the repository and as a packaged standalone binary.

## Architectural layers

### 1. Command layer

Responsible for:

- argument parsing
- shared flags such as `--format`
- exit-code policy
- writing JSON/text output
- turning domain errors into stable machine-readable responses

This layer must stay thin. It should delegate behavior to shared config, registry, search, and retrieval services.

### 2. Configuration layer

Responsible for:

- resolving the global config directory under `~/.config/search-paper-cli/`
- loading `config.yaml` first and `config.yml` only as a compatibility fallback
- decoding lowercase snake_case YAML keys mapped to the supported `SEARCH_PAPER_*` settings
- merging process environment over config file values per key
- ignoring legacy `.env` sources (`SEARCH_PAPER_ENV_FILE`, cwd `.env`, repo-root `.env`)
- emitting secret-safe diagnostics to stderr without corrupting JSON stdout

The configuration layer is the single source of truth for runtime settings used by `sources`, `search`, and retrieval.

### 3. Source registry and capability model

Responsible for:

- stable source ids
- whether a source is enabled
- per-source capabilities for `search`, `download`, and `read`
- disablement reasons for gated sources such as IEEE and ACM

The registry is the canonical source of truth for source availability across `sources`, `search`, and retrieval entrypoints.

### 4. Domain model

The core shared record is a normalized `Paper` model. In-memory fields may remain strongly typed, but default JSON output must preserve a stable downstream contract for agents.

Key invariants:

- every paper record has a stable `source`
- unified search returns one normalized record shape across providers
- source-specific metadata may exist, but the top-level contract remains stable

### 5. Connector layer

Each academic provider is implemented behind a shared source interface.

Connector responsibilities:

- request construction
- retries/backoff when needed
- response parsing
- source-native search/download/read behavior
- honoring endpoint overrides from the merged runtime config

Shared orchestration responsibilities must stay out of connectors.

### 6. Search orchestration layer

Responsible for:

- selecting sources from the registry
- bounded concurrent fan-out across selected sources
- partial-success handling
- per-source counts and errors
- cross-source deduplication

Deduplication invariant:

1. DOI
2. normalized title + authors
3. paper id

Duplicate survivor selection must be deterministic.

### 7. Retrieval orchestration layer

Responsible for:

- source-native download/read routing
- save-path handling
- machine-readable retrieval result states
- OA-first fallback sequencing
- exposing stage-level attempt details so validators can distinguish why a retrieval succeeded, skipped, or failed

Fallback invariant:

1. primary source-native retrieval
2. repository rediscovery through `openaire`, `core`, `europepmc`, and `pmc`
3. Unpaywall DOI lookup
4. optional Sci-Hub

### 8. Packaging and skill surface

Responsible for:

- producing release outputs under `dist/`
- keeping the standalone binary usable outside the repository
- keeping the retained `skills/search-paper` surface aligned with direct CLI usage rather than wrapper-mediated runtime config injection

Minimum deliverables remain:

- standalone Linux amd64 binary named `search-paper-cli`
- compressed archive containing that binary

## Validation-relevant invariants

- default stdout is parseable JSON unless `--format text` is requested
- warnings go to stderr
- `sources` ordering and schema are deterministic
- global YAML config plus process env are the only runtime configuration inputs
- built artifacts work outside the repository root
- direct `get --as pdf` / `get --as text` flows consume the same merged config seen by `sources` and `search`

## Response contract

Workers must follow the shared response contract in `output-contract.md`.
