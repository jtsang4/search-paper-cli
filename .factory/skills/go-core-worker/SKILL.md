---
name: go-core-worker
description: Build and verify core Go CLI foundations, global config loading, registry behavior, and search orchestration.
---

# Go Core Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for features that establish or modify:

- Go module and package structure
- root CLI commands and shared output formatting
- global config/env loading and merge behavior
- source registry/capability reporting
- paper model and unified search orchestration

## Required Skills

None.

## Work Procedure

1. Read `mission.md`, `AGENTS.md`, `.factory/library/architecture.md`, `.factory/library/environment.md`, and the assigned feature.
2. Identify the exact contract assertions the feature fulfills and restate them in your notes before editing.
3. Write failing tests first. Prefer:
   - unit tests for global-config discovery and env-over-config merge logic
   - CLI integration tests for warnings-on-stderr and source gating output
   - deterministic endpoint-override tests when same-key precedence must be proved black-box
4. Run the new tests to confirm they fail for the intended reason.
5. Implement the minimal production code to satisfy the failing tests while keeping config loading centralized in `internal/config`.
6. Remove legacy runtime `.env` discovery completely when the feature requires it; do not leave compatibility fallbacks unless the feature explicitly says to.
7. Run focused tests again until green.
8. Run manifest validators relevant to the change:
   - `.factory/services.yaml` `typecheck`
   - `.factory/services.yaml` `lint`
   - `.factory/services.yaml` `test`
9. Perform manual CLI smoke verification for the changed command surface using temp `HOME` directories and direct `search-paper-cli` or `go run` invocation. Do not use wrapper scripts.
10. Ensure warnings stay on stderr, JSON mode remains parseable, and no secrets appear in command output.
11. Create the feature commit. If `git commit` fails only because author identity is missing, retry with `git -c user.name="Droid" -c user.email="local@factory.invalid" commit ...` instead of changing git config.
12. In the handoff, include concrete command output observations, tests added, and any config-key mapping or precedence nuances discovered.

## Example Handoff

```json
{
  "salientSummary": "Replaced legacy `.env` discovery with global YAML config loading, added per-key env override semantics, and updated CLI/config tests to prove yaml-over-yml precedence plus ignored legacy `.env` inputs.",
  "whatWasImplemented": "Centralized config loading now resolves `~/.config/search-paper-cli/config.yaml` with `config.yml` fallback, maps lowercase YAML keys to the supported `SEARCH_PAPER_*` settings, merges process env per key, and ignores legacy `.env` inputs. Added failing tests first for malformed config warnings, blank/unknown values, same-key env precedence, and gated-source visibility in `sources`.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "go test ./internal/config -run 'Test(GlobalConfig|EnvMerge|LegacyEnvIgnored)' -count=1",
        "exitCode": 0,
        "observation": "Focused config tests passed, including yaml-over-yml precedence and ignored legacy `.env` inputs."
      },
      {
        "command": "GOMAXPROCS=8 go test -run '^$' -p 8 ./...",
        "exitCode": 0,
        "observation": "Compilation/typecheck passed across all packages."
      },
      {
        "command": "test -z "$(gofmt -l .)"",
        "exitCode": 0,
        "observation": "Formatting is clean."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Ran direct `search-paper-cli sources --format json` with temp HOME/config files and no wrapper commands.",
        "observed": "`ieee` and `acm` gating reflected the merged env/global-config contract and warnings stayed on stderr."
      }
    ]
  },
  "tests": {
    "added": [
      {
        "file": "internal/config/config_test.go",
        "cases": [
          {
            "name": "TestGlobalConfigYAMLPrecedence",
            "verifies": "config.yaml wins over config.yml and legacy `.env` inputs are ignored."
          },
          {
            "name": "TestSameKeyEnvOverrideUsesEnvEndpoint",
            "verifies": "A deterministic same-key endpoint override uses the env value rather than the config-file value."
          }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- The feature requires a product-level decision about YAML key names or config semantics not already stated in mission artifacts.
- A contract assertion can only be satisfied by changing release/skill packaging rather than core config behavior.
- Live validation depends on credentials or upstream access that are unavailable.
