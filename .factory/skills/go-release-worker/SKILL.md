---
name: go-release-worker
description: Build, package, and validate release artifacts and outside-repo CLI flows.
---

# Go Release Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for features that implement or modify:

- release packaging
- artifact layout under `dist/`
- outside-repo smoke validation
- build/release automation and artifact parity

## Required Skills

None.

## Work Procedure

1. Read the assigned feature plus `.factory/library/architecture.md`, `.factory/library/user-testing.md`, and the release-related contract assertions it fulfills.
2. Write failing tests or release validation scripts first where practical (for example artifact naming/layout checks or CLI smoke tests against built binaries).
3. Implement packaging/build logic that produces the required `dist/` deliverables.
4. Run repository validators before packaging work is considered done:
   - `typecheck`
   - `lint`
   - `test`
   - `build`
5. Produce the required artifact outputs under `dist/`.
6. Validate the built artifact outside the repository root using real CLI invocations.
7. Capture artifact paths, exact smoke commands, and outside-repo observations in the handoff.

## Example Handoff

```json
{
  "salientSummary": "Implemented release packaging for a Linux amd64 standalone binary and tarball, then validated the artifact outside the repo with a real CLI search-to-retrieval smoke flow.",
  "whatWasImplemented": "Added release build automation that writes a standalone `search-paper-cli` binary and compressed archive under `dist/`, plus smoke validation that executes the built binary from a temporary directory. Verified the artifact preserves env loading rules and machine-readable output outside the source tree.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "GOMAXPROCS=8 go test -count=1 -p 8 ./...",
        "exitCode": 0,
        "observation": "Full test suite passed before packaging."
      },
      {
        "command": "GOMAXPROCS=8 go build -o dist/search-paper-cli_linux_amd64/search-paper-cli ./cmd/search-paper-cli",
        "exitCode": 0,
        "observation": "Standalone Linux amd64 binary built successfully."
      },
      {
        "command": "tar -czf dist/search-paper-cli_linux_amd64.tar.gz -C dist/search-paper-cli_linux_amd64 search-paper-cli",
        "exitCode": 0,
        "observation": "Compressed archive created successfully."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Copied the built binary to a temporary directory outside the repo and ran help, sources, and a smoke search/retrieval flow.",
        "observed": "The artifact executed successfully outside the repository and preserved the same JSON-first CLI contract."
      }
    ]
  },
  "tests": {
    "added": [
      {
        "file": "internal/release/release_test.go",
        "cases": [
          {
            "name": "TestArtifactNamesAndLayout",
            "verifies": "Packaging produces the expected binary and archive under dist."
          }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- The agreed deliverable format needs to change.
- Outside-repo smoke validation fails due to environment limitations that workers cannot repair.
- Packaging requires credentials, signing, or publishing decisions that need user input.
