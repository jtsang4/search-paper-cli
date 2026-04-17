---
name: go-release-worker
description: Align release artifacts, skill packaging, direct-run workflow docs, and outside-repo validation for the Go CLI.
---

# Go Release Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for features that implement or modify:

- release packaging
- artifact layout under `dist/`
- outside-repo smoke validation
- skill packaging / helper assets
- user-facing direct-run workflow docs and validation assets

## Required Skills

None.

## Work Procedure

1. Read the assigned feature plus `.factory/library/architecture.md`, `.factory/library/environment.md`, `.factory/library/user-testing.md`, and the release-related contract assertions it fulfills.
2. Identify every shipped guidance or helper asset affected by the feature (`README.md`, repo `AGENTS.md`, skill docs, helper scripts, release tests, `.factory/validation` flow definitions) before editing.
3. Write failing tests or validation assets first where practical, especially for outside-repo artifact behavior and installed-skill-style direct CLI usage.
4. Implement the minimal changes needed to make the shipped workflow plain `search-paper-cli ...` with global config plus per-key env overrides.
5. Remove wrapper-driven runtime behavior when the feature calls for it; if any bootstrap helper remains, keep it install/locate-only and verify it does not inject runtime config.
6. Update stale validation assets and documentation consistently so no shipped guidance contradicts the new workflow.
7. Run repository validators before the feature is considered done:
   - `typecheck`
   - `lint`
   - `test`
   - `build`
8. Produce or reuse the required artifact outputs under `dist/` and validate the built artifact outside the repository root using direct CLI invocations with temp `HOME`.
9. Perform at least one installed-skill-style smoke check from a copied skill context using plain `search-paper-cli` commands, not the removed wrapper path.
10. Create the feature commit. If `git commit` fails only because author identity is missing, retry with `git -c user.name="Droid" -c user.email="local@factory.invalid" commit ...` instead of changing git config.
11. Capture exact artifact paths, grep/doc cleanup evidence, direct-run commands, and outside-repo observations in the handoff.

## Example Handoff

```json
{
  "salientSummary": "Removed the wrapper-driven skill flow, updated shipped guidance to direct `search-paper-cli` usage, and validated both the standalone artifact and an installed-skill-style context with temp HOME global config.",
  "whatWasImplemented": "Deleted the runtime wrapper script, rewrote README/AGENTS/skill references to describe `~/.config/search-paper-cli/config.yaml` plus per-key env overrides, updated stale validation flow definitions, and retargeted release tests to outside-repo direct CLI behavior. Verified the built artifact and copied skill context use plain `search-paper-cli ...` commands with no `SEARCH_PAPER_ENV_FILE` or skill-local `.env` coupling.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "go test ./internal/release -run 'Test(Artifact|Skill)' -count=1",
        "exitCode": 0,
        "observation": "Release and skill tests passed with direct-run global-config expectations."
      },
      {
        "command": "GOMAXPROCS=8 go build ./...",
        "exitCode": 0,
        "observation": "All packages and the CLI binary build successfully."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Ran the built binary from an outside-repo temp directory with temp HOME global config and executed `sources`, `search`, and `get --as pdf`.",
        "observed": "The direct CLI surface worked without wrapper assets, and stdout/stderr behavior stayed machine-readable."
      },
      {
        "action": "Copied the skill directory to a temp location and followed the updated docs using plain `search-paper-cli` commands.",
        "observed": "The installed-skill context no longer required wrapper scripts or skill-local `.env` scaffolding."
      }
    ]
  },
  "tests": {
    "added": [
      {
        "file": "internal/release/release_test.go",
        "cases": [
          {
            "name": "TestArtifactUsesGlobalConfigOutsideRepo",
            "verifies": "The standalone binary honors the new global-config model from an outside-repo context."
          }
        ]
      },
      {
        "file": "internal/release/skill_test.go",
        "cases": [
          {
            "name": "TestInstalledSkillContextUsesDirectCLI",
            "verifies": "The retained skill surface relies on plain `search-paper-cli` invocation rather than a wrapper runtime path."
          }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- The agreed deliverable format or skill packaging shape needs to change materially.
- Outside-repo smoke validation fails due to environment limitations that workers cannot repair.
- A doc/skill cleanup requirement conflicts with earlier mission guidance or introduces broader scope than the feature description allows.
