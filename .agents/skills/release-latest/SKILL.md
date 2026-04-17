---
name: release-latest
description: Prepare and publish a new search-paper-cli release when the caller provides a version like v1.1.0.
disable-model-invocation: true
---

# Release Latest

## When to Use

Use this skill only for a full manual `search-paper-cli` release when the caller wants the repository updated, validated, tagged, pushed, and published on GitHub.

Do not update unrelated documentation or `README.md` as part of this workflow.

## Required Input

The caller must provide exactly one release version in `vX.Y.Z` form, for example `v1.1.0`.

Stop immediately if the version is missing or does not match `^v[0-9]+\.[0-9]+\.[0-9]+$`.

## Hard Blockers

Stop on any blocker. Do not continue past any failed check or partial release state trigger.

Hard blockers include:

- missing or invalid caller-supplied version
- dirty working tree
- branch or release intent not appropriate for publishing
- target tag already exists locally or on `origin`
- `gh auth status` failure
- validator failure
- packaging failure
- missing expected dist artifacts
- push failure
- GitHub release creation failure

## Release Procedure

1. Verify the caller supplied `<VERSION>` in `vX.Y.Z` form. Stop if missing or invalid.
2. Verify the working tree is clean with `git status --short`. Stop if anything is modified or untracked.
3. Verify the current branch and release intent are appropriate for publishing before making changes.
4. Verify the tag does not already exist locally or remotely:
   - `git rev-parse -q --verify "refs/tags/<VERSION>"`
   - `git ls-remote --tags origin "<VERSION>"`
5. Verify GitHub authentication before changing release state:
   - `gh auth status`
6. Update `internal/cli/app.go` so:
   - `const defaultVersion = "search-paper-cli <VERSION>"`
7. Run the required repository validators from `AGENTS.md` and stop on any failure:
   - `GOMAXPROCS=8 go test -run '^$' -p 8 ./...`
   - `test -z "$(gofmt -l .)"`
   - `GOMAXPROCS=8 go test -count=1 -p 8 ./...`
   - `GOMAXPROCS=8 go build ./...`
8. Run release packaging:
   - `./scripts/package-release.sh`
9. Verify both expected artifacts exist:
   - `dist/search-paper-cli_linux_amd64/search-paper-cli`
   - `dist/search-paper-cli_linux_amd64.tar.gz`
10. Review `git status` and the staged diff before committing.
11. Commit only the release changes with:
   - `git commit -m "release: set CLI version to <VERSION>"`
12. Create the tag:
   - `git tag "<VERSION>"`
13. Push the release commit and tag to `origin`. Stop if either push fails.
14. Create the GitHub release and upload both artifacts with `gh release create`.

## Output

Report:

- commit SHA
- tag
- GitHub release URL
- artifact paths:
  - `dist/search-paper-cli_linux_amd64/search-paper-cli`
  - `dist/search-paper-cli_linux_amd64.tar.gz`

If any blocker occurs, stop immediately and report the exact failing step and command.
