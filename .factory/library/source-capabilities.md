# Source Capabilities

Mission-level grouping of source behavior classes.

**What belongs here:** source categories, expected capability patterns, special validation notes.

---

## Direct full-text capable sources

Expected to support source-native search and at least some successful download/read cases:

- arXiv
- bioRxiv
- medRxiv
- IACR
- Semantic Scholar (OA-only cases)
- PMC
- Europe PMC
- CORE
- DOAJ
- BASE
- Zenodo
- HAL

## Metadata / info-only sources

Expected to support search, but direct full-text is informational or unsupported:

- PubMed
- Crossref
- OpenAlex
- Google Scholar

## Hard-unsupported retrieval sources

Expected to fail download/read explicitly as unsupported:

- dblp
- OpenAIRE
- Unpaywall direct retrieval

## Record-dependent / best-effort sources

May succeed only when a public file is actually available:

- SSRN
- CiteSeerX
- DOAJ
- BASE
- Zenodo
- HAL
- Semantic Scholar OA cases

## Gated skeleton sources

Enabled only when keys are present, but still unimplemented for retrieval:

- IEEE
- ACM

## Special fallback-only behavior

- Unpaywall is DOI-centric metadata + OA URL resolution, not direct full-text retrieval.
- Sci-Hub is optional, explicitly opt-in, and always last in the fallback chain.

## Provider-specific search notes

- OpenAIRE search behavior is format-sensitive. The legacy `https://api.openaire.eu/search/publications` endpoint returns XML by default; JSON callers must request `format=json`, and the reference implementation prefers the XML-first `/search/researchProducts` flow before any legacy fallback.
