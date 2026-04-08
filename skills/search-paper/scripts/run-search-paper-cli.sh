#!/usr/bin/env sh
set -eu

usage() {
  cat <<'USAGE'
Usage: sh scripts/run-search-paper-cli.sh <command> [args...]

Run search-paper-cli with SEARCH_PAPER_ENV_FILE pointing to this skill directory's .env file.
Required configuration:
  SEARCH_PAPER_UNPAYWALL_EMAIL

This skill only recognizes SEARCH_PAPER_-prefixed variables.
Example:
  SEARCH_PAPER_UNPAYWALL_EMAIL=you@example.com

If the required configuration is missing, create .env next to .env.example in this skill directory and rerun the command.
USAGE
}

if [ "$#" -eq 0 ]; then
  usage >&2
  exit 2
fi

if [ "${1-}" = "--skill-help" ]; then
  usage
  exit 0
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SKILL_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
ENV_FILE="$SKILL_DIR/.env"
ENV_EXAMPLE="$SKILL_DIR/.env.example"
ENSURE_SCRIPT="$SKILL_DIR/scripts/ensure-search-paper-cli.sh"
REQUIRED_VAR="SEARCH_PAPER_UNPAYWALL_EMAIL"

read_env_file_value() {
  key="$1"
  file="$2"
  awk -v key="$key" '
    /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
    {
      line = $0
      sub(/^[[:space:]]*export[[:space:]]+/, "", line)
      pos = index(line, "=")
      if (pos <= 1) {
        next
      }
      current_key = substr(line, 1, pos - 1)
      current_value = substr(line, pos + 1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", current_key)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", current_value)
      if (current_key != key) {
        next
      }
      if (current_value ~ /^".*"$/ || current_value ~ /^'.*'$/) {
        current_value = substr(current_value, 2, length(current_value) - 2)
      }
      print current_value
      exit
    }
  ' "$file"
}

require_value() {
  key="$1"
  if [ -n "$(printenv "$key" 2>/dev/null || true)" ]; then
    return 0
  fi
  if [ -f "$ENV_FILE" ] && [ -n "$(read_env_file_value "$key" "$ENV_FILE")" ]; then
    return 0
  fi

  printf 'error: missing required environment variable %s.\n' "$key" >&2
  printf 'error: this skill only recognizes SEARCH_PAPER_-prefixed variables; legacy or fuzzy-matched names are ignored.\n' >&2
  printf 'error: example: %s=you@example.com\n' "$key" >&2
  printf 'error: create %s from %s and fill in the required values.\n' "$ENV_FILE" "$ENV_EXAMPLE" >&2
  exit 1
}

require_value "$REQUIRED_VAR"
CLI_PATH=$(sh "$ENSURE_SCRIPT")

if [ -f "$ENV_FILE" ]; then
  exec env SEARCH_PAPER_ENV_FILE="$ENV_FILE" "$CLI_PATH" "$@"
fi

exec "$CLI_PATH" "$@"
