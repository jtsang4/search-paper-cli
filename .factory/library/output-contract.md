# Output Contract

Shared machine-readable CLI response rules.

**What belongs here:** top-level envelopes, state vocabulary, and exit-code mapping.

---

## Global rules

- Default command output is JSON on stdout.
- `--format text` may render human-readable output, but stderr must still be reserved for warnings/diagnostics.
- Non-fatal warnings never pollute JSON stdout.

## Top-level status

- Success payloads use `"status": "ok"`.
- Failure payloads use `"status": "error"`.

## Error envelope

Failure responses should use:

```json
{
  "status": "error",
  "error": {
    "code": "string_code",
    "message": "human-readable summary",
    "details": {}
  }
}
```

## Search success envelope

```json
{
  "status": "ok",
  "query": "string",
  "requested_sources": ["semantic", "crossref"],
  "used_sources": ["semantic", "crossref"],
  "invalid_sources": ["typo-source"],
  "source_results": {
    "semantic": 3,
    "crossref": 2
  },
  "errors": {
    "crossref": "upstream failure message"
  },
  "total": 4,
  "papers": []
}
```

Rules:

- `source_results` is pre-dedupe count per source.
- `total` is post-dedupe merged count.
- `invalid_sources` must be a machine-readable field when mixed valid/invalid source ids are supplied.

## Source listing success envelope

```json
{
  "status": "ok",
  "sources": [
    {
      "id": "arxiv",
      "enabled": true,
      "disable_reason": "",
      "capabilities": {
        "search": "supported",
        "download": "supported",
        "read": "supported"
      }
    }
  ]
}
```

Capability state values:

- `supported`
- `record_dependent`
- `informational`
- `unsupported`
- `gated`

## Retrieval success envelope

```json
{
  "status": "ok",
  "operation": "download",
  "state": "downloaded",
  "source": "arxiv",
  "paper_id": "1234.5678",
  "path": "/tmp/out/paper.pdf",
  "attempts": []
}
```

```json
{
  "status": "ok",
  "operation": "read",
  "state": "extracted",
  "source": "arxiv",
  "paper_id": "1234.5678",
  "content": "text...",
  "attempts": []
}
```

Allowed retrieval state values:

- `downloaded`
- `extracted`
- `downloaded_but_not_extractable`
- `informational`
- `unsupported`
- `not_found`
- `failed`

Rules:

- A success response with `path` must point to a real file.
- Unsupported or informational retrieval results must not pretend to be `downloaded` or `extracted`.
- Fallback retrieval must include ordered `attempts` with stage names such as `primary`, `repositories`, `unpaywall`, `scihub`.

## Exit codes

- `0`: successful command execution, including legitimate zero-result search
- `2`: invalid usage or invalid input
- `3`: unsupported capability, gated source, or missing required config for the requested capability
- `4`: runtime/upstream failure after valid invocation
