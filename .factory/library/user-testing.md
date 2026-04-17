# User Testing

Validation surface, tool choices, and resource guidance.

**What belongs here:** how validators should exercise the product through real user-facing boundaries.

---

## Validation Surface

Primary surface: direct CLI execution.

Validators should verify:

- global-config discovery from temp `HOME`
- `config.yaml` vs `config.yml` precedence
- per-key env-over-config merge semantics
- absence of legacy `.env` runtime behavior
- direct `search-paper-cli` usage from repo and outside-repo artifact contexts
- `sources`, `search`, `get --as pdf`, and `get --as text` behavior under the same merged config
- stdout JSON/text behavior and stderr isolation for warnings
- file creation in requested save paths for retrieval flows

No browser or TUI validation is required for this mission.

## Validation Tools

- primary: shell/CLI execution
- deterministic network behavior: Go tests with `httptest` or equivalent mock endpoints
- artifact validation: built standalone binary executed from an outside-repo temp directory

## Validation Concurrency

- direct CLI / deterministic validation: up to 5 concurrent validators
- network-dependent smoke validation: up to 2 concurrent validators

Reasoning:

- host has 16 CPU cores and ample RAM
- CLI validation is lightweight, but hermetic temp-home artifact flows still spawn build/test work
- keeping concurrency at or below 5 avoids noisy cross-test interference while preserving throughput

## Validation Environment Rules

- Always use a temporary `HOME` for config-loading checks.
- Keep the working directory outside the repository when validating outside-repo artifact behavior.
- Do not set `SEARCH_PAPER_ENV_FILE` in new validation flows unless explicitly testing that it is ignored.
- Do not rely on cwd `.env`, repository-root `.env`, or skill-local `.env` as setup inputs.
- Preserve machine-readable stdout; warnings and diagnostics must stay on stderr.

## Evidence Expectations

Validators should capture:

- exact command line invoked
- exit code
- stdout/stderr
- temp `HOME` / working-directory arrangement when relevant
- created file paths
- artifact paths for release validation
- fallback stage/attempt details for retrieval flows

## Flow Validator Guidance: CLI

- Prefer direct `search-paper-cli` execution for observable behavior.
- Use the built CLI artifact when validating outside-repo behavior.
- Use dedicated temp directories per validator group under `.tmp/user-testing/` to avoid interference.
- Update stale validation flow definitions that still reference wrapper commands or legacy `.env` discovery before relying on them for milestone validation.
