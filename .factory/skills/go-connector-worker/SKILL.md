---
name: go-connector-worker
description: Build and verify source connectors, retrieval behavior, and fallback flows for the Go CLI.
---

# Go Connector Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for features that implement or modify:

- source connector behavior
- search normalization per provider
- source-native download/read logic
- fallback orchestration
- record-dependent, info-only, or gated retrieval behavior

## Required Skills

None.

## Work Procedure

1. Read the assigned feature plus `.factory/library/architecture.md`, `.factory/library/environment.md`, `.factory/library/source-capabilities.md`, and `.factory/library/user-testing.md`.
2. Map the feature to specific source behavior classes before writing code.
3. Write failing tests first:
   - fixture-backed parser/normalization tests
   - `httptest`-based request/response tests
   - retrieval/fallback orchestration tests that verify winning stage and side effects
4. Confirm the tests fail before implementing.
5. Implement the connector or retrieval behavior with shared helpers where appropriate, but keep source-specific retry/backoff/parsing logic in the connector layer.
6. Honor save-path rules and ensure unsupported/error flows do not create stray files.
7. Run focused tests, then run manifest validators:
   - `typecheck`
   - `lint`
   - `test`
8. If live-network smoke checks are feasible and safe, run the smallest representative smoke check needed for the feature. If live credentials are unavailable, document the deterministic coverage instead of guessing.
9. Create the feature commit. If `git commit` fails only because author identity is missing, retry with `git -c user.name="Droid" -c user.email="local@factory.invalid" commit ...` instead of changing git config.
10. Record the exact capability class and any upstream volatility in the handoff.

## Example Handoff

```json
{
  "salientSummary": "Implemented Semantic Scholar, Unpaywall, and fallback retrieval behavior with machine-readable attempt details. Added fixture and httptest coverage first, then verified save-path handling and unsupported-source behavior.",
  "whatWasImplemented": "Added connector implementations for selected sources, normalized their paper output into the shared model, and wired source-native download/read plus OA-first fallback orchestration. Added explicit success, informational, unsupported, and record-dependent retrieval states so the CLI no longer returns ambiguous success strings.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "go test ./... -run 'TestSemanticSearch|TestUnpaywallLookup|TestFallbackOrdering|TestReadStates'",
        "exitCode": 0,
        "observation": "Focused connector and retrieval tests passed, including repository-before-Unpaywall-before-Sci-Hub ordering."
      },
      {
        "command": "GOMAXPROCS=8 go test -count=1 -p 8 ./...",
        "exitCode": 0,
        "observation": "Full test suite passed after connector integration."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Ran a local CLI smoke flow against fixture-backed test cases and one optional live smoke case.",
        "observed": "Download success produced a real PDF inside the requested directory; unsupported sources returned explicit non-success JSON without creating files."
      }
    ]
  },
  "tests": {
    "added": [
      {
        "file": "internal/connectors/semantic/semantic_test.go",
        "cases": [
          {
            "name": "TestYearFilterSupportsSingleYearAndRange",
            "verifies": "Semantic Scholar year handling accepts both single-year and range-like inputs."
          }
        ]
      },
      {
        "file": "internal/retrieval/fallback_test.go",
        "cases": [
          {
            "name": "TestRepositoryFallbackShortCircuitsSciHub",
            "verifies": "Repository fallback wins before later stages and reports the winning stage."
          }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- A connector requires a new product-level decision about capability shape or JSON contract.
- Upstream instability makes live validation impossible and the contract needs to be revised.
- A source should be split into a new feature because the assigned scope is too large for one worker session.
