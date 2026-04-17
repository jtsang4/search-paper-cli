# Environment

Environment variables, global configuration, external dependencies, and setup notes.

**What belongs here:** required env vars, supported global-config keys, precedence rules, secret-handling rules.
**What does NOT belong here:** test/build commands or port bindings.

---

## Global config location

Primary config file:

- `~/.config/search-paper-cli/config.yaml`

Compatibility fallback:

- `~/.config/search-paper-cli/config.yml`

If both files exist, `config.yaml` wins.

## Supported global-config keys

Lowercase snake_case keys mapped to the existing `SEARCH_PAPER_*` settings:

- `unpaywall_email`
- `core_api_key`
- `semantic_scholar_api_key`
- `google_scholar_proxy_url`
- `doaj_api_key`
- `zenodo_access_token`
- `ieee_api_key`
- `acm_api_key`
- `arxiv_base_url`
- `openaire_base_url`
- `openaire_legacy_base_url`
- `core_base_url`
- `europepmc_base_url`
- `pmc_search_url`
- `pmc_summary_url`
- `unpaywall_base_url`

Unknown keys should be ignored safely.

## Environment precedence

- Only process environment variables with the exact `SEARCH_PAPER_*` names are recognized.
- Process env wins per key over the global config file.
- Explicitly empty env values still count as overrides and must block fallback to file values for that key.
- Effective runtime config is the per-key merge of process env and global config.
- Legacy `.env` inputs are no longer part of runtime config loading:
  - `SEARCH_PAPER_ENV_FILE`
  - cwd `.env`
  - repository-root `.env`

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

These are intended for deterministic local testing and mock servers. They remain optional for normal runtime use.

## Secret handling

- Never commit real credentials.
- Never print raw secret values in logs, stdout, stderr, or handoffs.
- Use temp `HOME` directories for tests that exercise global-config loading so host credentials are never consulted accidentally.
