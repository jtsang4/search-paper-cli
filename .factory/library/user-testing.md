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

## Evidence Expectations

Validators should capture:

- command line invoked
- exit code
- stdout/stderr
- created file paths
- artifact paths for release validation
- stage/attempt details for fallback retrieval flows
