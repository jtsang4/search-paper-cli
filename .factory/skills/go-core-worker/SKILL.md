---
name: go-core-worker
description: Build and verify core Go CLI foundations, config, registry, and search orchestration.
---

# Go Core Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for features that establish or modify:

- Go module and package structure
- root CLI commands and shared output formatting
- config/env loading
- source registry/capability reporting
- paper model and unified search orchestration

## Required Skills

None.

## Work Procedure

1. Read `mission.md`, `AGENTS.md`, `.factory/library/architecture.md`, `.factory/library/environment.md`, and the assigned feature.
2. Identify the exact contract assertions the feature fulfills and restate them in your notes before editing.
3. Write failing tests first. Prefer:
   - unit tests for env/config logic
   - CLI integration tests for command output and exit codes
   - fixture-backed tests for search aggregation and dedupe
4. Run the new tests to confirm they fail for the intended reason.
5. Implement the minimal production code to satisfy the failing tests while preserving shared orchestration and avoiding duplicated command logic.
6. Run focused tests again until green.
7. Run manifest validators relevant to the change:
   - `.factory/services.yaml` `typecheck`
   - `.factory/services.yaml` `lint`
   - `.factory/services.yaml` `test`
8. Perform manual CLI smoke verification for the changed command surface using either `go test` integration cases or `go run`/built binary execution.
9. Ensure warnings stay on stderr and JSON mode remains parseable.
10. Create the feature commit. If `git commit` fails only because author identity is missing, retry with `git -c user.name="Droid" -c user.email="local@factory.invalid" commit ...` instead of changing git config.
11. In the handoff, include concrete command output observations, tests added, and any source behaviors or contract nuances discovered.

## Example Handoff

```json
{
  "salientSummary": "Implemented the root CLI, version/help surfaces, env loading, and the source registry JSON/text renderers. Added failing CLI/config tests first, then wired shared formatting and deterministic source ordering until all scoped and repo validators passed.",
  "whatWasImplemented": "Created the Go module entrypoint, root command tree, shared formatter, env loader for prefixed environment variables, and a registry layer that exposes enabled state plus per-capability status for each source. Added CLI integration tests for help/version/sources output and config precedence behavior.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "go test ./... -run 'TestRootHelp|TestVersion|TestSourcesJSON|TestEnvPrecedence'",
        "exitCode": 0,
        "observation": "Focused CLI and config tests passed; JSON stdout remained parseable and warnings stayed on stderr."
      },
      {
        "command": "GOMAXPROCS=8 go test -run '^$' -p 8 ./...",
        "exitCode": 0,
        "observation": "Compilation/typecheck passed across all packages."
      },
      {
        "command": "test -z \"$(gofmt -l .)\"",
        "exitCode": 0,
        "observation": "Formatting is clean."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Ran `go run ./cmd/search-paper-cli --help` and `go run ./cmd/search-paper-cli sources` in a clean env.",
        "observed": "Help listed the expected commands; `sources` emitted valid JSON with deterministic ordering and explicit IEEE/ACM disabled reasons."
      }
    ]
  },
  "tests": {
    "added": [
      {
        "file": "internal/config/config_test.go",
        "cases": [
          {
            "name": "TestPrefixedEnvLoadsValues",
            "verifies": "Prefixed env vars load directly and preserve explicit empty values."
          }
        ]
      },
      {
        "file": "internal/cli/root_test.go",
        "cases": [
          {
            "name": "TestSourcesJSONOutput",
            "verifies": "Default sources output is parseable JSON with deterministic ordering."
          }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- The feature requires source behavior that depends on connectors not yet implemented.
- The contract requires a CLI shape that conflicts with earlier feature decisions.
- Live validation depends on credentials or upstream access that are unavailable.
