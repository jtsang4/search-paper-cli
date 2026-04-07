# Environment

Environment variables, external dependencies, and setup notes.

**What belongs here:** required env vars, optional keys, `.env` loading behavior, secret-handling rules.
**What does NOT belong here:** test/build commands or port bindings.

---

## Required for specific capabilities

- `PAPER_SEARCH_MCP_UNPAYWALL_EMAIL`
  - Required to activate Unpaywall DOI/OA lookup behavior.
  - Legacy alias: `UNPAYWALL_EMAIL`
- `PAPER_SEARCH_MCP_CORE_API_KEY`
  - Recommended for CORE reliability and rate limits.
  - Legacy alias: `CORE_API_KEY`

## Optional integrations

- `PAPER_SEARCH_MCP_SEMANTIC_SCHOLAR_API_KEY`
- `PAPER_SEARCH_MCP_GOOGLE_SCHOLAR_PROXY_URL`
- `PAPER_SEARCH_MCP_DOAJ_API_KEY`
- `PAPER_SEARCH_MCP_ZENODO_ACCESS_TOKEN`
- `PAPER_SEARCH_MCP_IEEE_API_KEY`
- `PAPER_SEARCH_MCP_ACM_API_KEY`

## `.env` behavior to preserve

1. `PAPER_SEARCH_MCP_ENV_FILE` wins when set.
2. Otherwise `./.env` in the current working directory wins.
3. Otherwise repository-root `.env` is used when running from within the source tree.
4. Discovery stops after the first existing file.
5. Prefixed `PAPER_SEARCH_MCP_*` variables override legacy aliases.
6. An explicitly empty prefixed value blocks fallback to the legacy alias.

## Secret handling

- Never commit `.env` or any real credentials.
- Never print secret values in logs, command output, or handoffs.
- If local credentials are available in `.env`, workers may use them for live smoke checks.
- A local ignored `.env` may already be pre-populated for this mission; treat it as runtime-only input, not source.
- Live-network validation must degrade gracefully when optional credentials are unavailable.
- Built-artifact validation outside the repository should rely on an explicit env file or the current working directory `.env`, not repository-root fallback.
