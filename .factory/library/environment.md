# Environment

Environment variables, external dependencies, and setup notes.

**What belongs here:** required env vars, optional keys, `.env` loading behavior, secret-handling rules.
**What does NOT belong here:** test/build commands or port bindings.

---

## Required for specific capabilities

- `SEARCH_PAPER_UNPAYWALL_EMAIL`
  - Required to activate Unpaywall DOI/OA lookup behavior.
- `SEARCH_PAPER_CORE_API_KEY`
  - Recommended for CORE reliability and rate limits.

## Optional integrations

- `SEARCH_PAPER_SEMANTIC_SCHOLAR_API_KEY`
- `SEARCH_PAPER_GOOGLE_SCHOLAR_PROXY_URL`
- `SEARCH_PAPER_DOAJ_API_KEY`
- `SEARCH_PAPER_ZENODO_ACCESS_TOKEN`
- `SEARCH_PAPER_IEEE_API_KEY`
- `SEARCH_PAPER_ACM_API_KEY`

## Optional endpoint overrides for deterministic testing

- `SEARCH_PAPER_ARXIV_BASE_URL`
- `SEARCH_PAPER_OPENAIRE_BASE_URL`
- `SEARCH_PAPER_OPENAIRE_LEGACY_BASE_URL`
- `SEARCH_PAPER_CORE_BASE_URL`
- `SEARCH_PAPER_EUROPEPMC_BASE_URL`
- `SEARCH_PAPER_PMC_SEARCH_URL`
- `SEARCH_PAPER_PMC_SUMMARY_URL`
- `SEARCH_PAPER_UNPAYWALL_BASE_URL`

These are intended for local deterministic testing and built-artifact validation against mock servers. They are optional, should not be required for normal runtime use, and should not be pointed at secret or internal services unless that usage is already explicitly approved for the mission.

## `.env` behavior to preserve

1. `SEARCH_PAPER_ENV_FILE` wins when set.
2. Otherwise `./.env` in the current working directory wins.
3. Otherwise repository-root `.env` is used when running from within the source tree.
4. Discovery stops after the first existing file.
5. Only `SEARCH_PAPER_*` variables are recognized.

## Secret handling

- Never commit `.env` or any real credentials.
- Never print secret values in logs, command output, or handoffs.
- If local credentials are available in `.env`, workers may use them for live smoke checks.
- A local ignored `.env` may already be pre-populated for this mission; treat it as runtime-only input, not source.
- Live-network validation must degrade gracefully when optional credentials are unavailable.
- Built-artifact validation outside the repository should rely on an explicit env file or the current working directory `.env`, not repository-root fallback.
