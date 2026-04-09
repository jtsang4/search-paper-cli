# Architecture

How the system works at a high level.

**What belongs here:** components, boundaries, data flow, invariants, release shape.
**What does NOT belong here:** low-level implementation notes or one-off bug lists.

---

## System overview

`search-paper-cli` is a single Go CLI that replaces the reference Python MCP/server + CLI split with one shared command-oriented application layer.

Primary command families:

- `sources`
- `search`
- `download`
- `read`
- `version`

The product is JSON-first by default, with `--format text` as an opt-in human-readable renderer.

## Architectural layers

### 1. Command layer

Responsible for:

- argument parsing
- shared flags such as `--format`
- exit-code policy
- writing JSON/text output
- turning domain errors into stable machine-readable responses

This layer must stay thin. It should delegate all real behavior to shared services.

### 2. Configuration layer

Responsible for:

- loading `.env` and process env
- prefixed env resolution
- source activation/gating
- secret-safe diagnostics

This layer must not leak credentials to stdout/stderr.

Source-tree env discovery may use repository-root `.env`; built-artifact execution outside the repo should rely on explicit env-file selection or the current working directory `.env`.

### 3. Source registry and capability model

Responsible for:

- stable source ids
- whether a source is enabled
- per-source capabilities for `search`, `download`, and `read`
- disablement reasons for gated sources such as IEEE and ACM

The registry is the canonical source of truth for source availability across `sources`, `search`, `download`, and `read`.

### 4. Domain model

The core shared record is a normalized `Paper` model. In-memory fields may remain strongly typed, but default JSON output must preserve a stable downstream contract for agents.

Key invariants:

- every paper record has a stable `source`
- unified search returns one normalized record shape across all providers
- source-specific metadata may exist, but the top-level contract is stable

### 5. Connector layer

Each academic provider is implemented behind a shared source interface.

Connector responsibilities:

- request construction
- retries/backoff when needed
- response parsing
- source-native search/download/read behavior

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

Fallback invariant:

1. primary source-native retrieval
2. repository rediscovery through `openaire`, `core`, `europepmc`, and `pmc`
3. Unpaywall DOI lookup
4. optional Sci-Hub

The retrieval layer should expose stage-level attempt details so validators and agents can tell why a retrieval succeeded or failed.

Repository rediscovery rules:

- use DOI first when available
- fall back to title-based rediscovery when DOI is absent
- skip the stage only when neither DOI nor title is available
- stop immediately when one repository stage produces a real downloadable file

### 8. Packaging layer

Responsible for producing release outputs under `dist/`.

Minimum deliverables for this mission:

- standalone Linux amd64 binary named `search-paper-cli`
- compressed archive containing that binary

Built artifacts must preserve the same CLI surface and env behavior as source-tree invocation.

## Source behavior classes

Workers should reason about sources in behavior classes rather than assuming every connector behaves alike:

- direct full-text sources
- metadata/info-only sources
- hard-unsupported retrieval sources
- record-dependent / best-effort retrieval sources
- gated skeleton sources

See `source-capabilities.md` for the mission-level grouping.

## Validation-relevant invariants

- default stdout is parseable JSON unless `--format text` is requested
- warnings go to stderr
- `sources` ordering and schema are deterministic
- mixed valid/invalid source selection is reported in machine-readable form
- successful download/read responses never point to non-existent files
- unsupported/error retrieval responses do not leave stray files
- built artifacts work outside the repository root

## Response contract

Workers must follow the shared response contract in `output-contract.md`.
