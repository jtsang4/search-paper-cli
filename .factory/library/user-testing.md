# User Testing

Validation surface, tool choices, and resource guidance.

**What belongs here:** how validators should exercise the product through real user-facing boundaries.

---

## Validation Surface

Primary surface: CLI via shell execution.

Validators should verify:

- exit codes
- stdout JSON/text behavior
- stderr isolation for warnings
- file creation in requested save paths
- env-file and env-var behavior
- built-artifact behavior outside the repo

No browser or TUI validation is required for this mission.

## Validation Tools

- primary: shell/cli execution
- deterministic HTTP behavior: Go tests with `httptest`
- optional smoke validation: real outbound HTTP against stable public sources

## Validation Concurrency

- deterministic CLI / fixture-backed validation: up to 8 concurrent validators
- network-dependent smoke validation: 2-4 concurrent validators max

Reasoning:

- host has 16 CPU cores and ample RAM
- CLI validation is lightweight
- external providers may rate-limit or vary in latency

## Network Validation Policy

- Default validation should be deterministic and fixture-heavy.
- Live smoke checks are required for selected stable flows and should use local credentials if available.
- High-volatility or anti-bot sources must not block the default validation path unless the user explicitly changes that expectation.
- If live Unpaywall DOI smoke returns upstream HTTP `422` despite valid local config, treat that as external instability and validate the configured-path contract with `go test ./internal/cli -run '^TestUnpaywallStandaloneLimits$' -count=1 -v` plus built-CLI clean-env and unsupported-retrieval checks instead of blocking the milestone.

## Evidence Expectations

Validators should capture:

- command line invoked
- exit code
- stdout/stderr
- created file paths
- artifact paths for release validation
- stage/attempt details for fallback retrieval flows

## Flow Validator Guidance: CLI

- Use the built CLI at `.tmp/user-testing/search-paper-cli` as the primary user surface when validating commands directly.
- Treat the repository checkout and built binary as read-only shared resources.
- Use a dedicated temp working directory and output directory per validator group under `.tmp/user-testing/` to avoid cross-test `.env`, output, or file-creation interference.
- Do not modify source files, `.env`, or shared validation artifacts outside your assigned flow report and evidence directory.
- Prefer direct CLI command execution for observable behavior; use targeted `go test` command(s) only when an assertion depends on deterministic internal orchestration that cannot be observed reliably from live network output alone.
- Keep stdout/stderr, exit codes, and any created files as evidence in the flow report.
