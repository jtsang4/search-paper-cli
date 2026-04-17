#!/usr/bin/env sh
set -eu

usage() {
  cat <<'USAGE'
Usage: sh scripts/ensure-search-paper-cli.sh

Resolve the search-paper-cli binary path.
This helper only locates or installs the binary; runtime configuration is handled by
search-paper-cli itself from process env plus ~/.config/search-paper-cli/config.yaml
(or config.yml when config.yaml is absent).
Resolution order:
1. SEARCH_PAPER_CLI_BIN when it points to an executable file
2. search-paper-cli from PATH
3. ./bin/search-paper-cli inside this skill directory
4. Install with `go install github.com/jtsang4/search-paper-cli/cmd/search-paper-cli@latest`
5. On linux/amd64 only, download the latest GitHub release artifact into ./bin/
USAGE
}

case "${1-}" in
  "")
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    printf 'error: unexpected argument: %s\n' "$1" >&2
    usage >&2
    exit 2
    ;;
esac

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SKILL_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
LOCAL_BIN_DIR="$SKILL_DIR/bin"
LOCAL_BIN_PATH="$LOCAL_BIN_DIR/search-paper-cli"
MODULE_PATH="github.com/jtsang4/search-paper-cli/cmd/search-paper-cli"

if [ -n "${SEARCH_PAPER_CLI_BIN-}" ]; then
  if [ -x "$SEARCH_PAPER_CLI_BIN" ]; then
    printf '%s\n' "$SEARCH_PAPER_CLI_BIN"
    exit 0
  fi
  printf 'warning: SEARCH_PAPER_CLI_BIN is not executable: %s\n' "$SEARCH_PAPER_CLI_BIN" >&2
fi

if command -v search-paper-cli >/dev/null 2>&1; then
  command -v search-paper-cli
  exit 0
fi

if [ -x "$LOCAL_BIN_PATH" ]; then
  printf '%s\n' "$LOCAL_BIN_PATH"
  exit 0
fi

printf 'warning: search-paper-cli was not found; installing the latest version\n' >&2
mkdir -p "$LOCAL_BIN_DIR"

if command -v go >/dev/null 2>&1; then
  if GOBIN="$LOCAL_BIN_DIR" go install "${MODULE_PATH}@latest"; then
    printf '%s\n' "$LOCAL_BIN_PATH"
    exit 0
  fi
  printf 'warning: go install failed; trying release artifact fallback\n' >&2
fi

os=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')
arch=$(uname -m 2>/dev/null)
case "$arch" in
  x86_64)
    arch="amd64"
    ;;
  amd64)
    arch="amd64"
    ;;
esac

if [ "$os" = "linux" ] && [ "$arch" = "amd64" ] && command -v tar >/dev/null 2>&1; then
  if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
    asset="search-paper-cli_${os}_${arch}.tar.gz"
    url="https://github.com/jtsang4/search-paper-cli/releases/latest/download/${asset}"
    tmp_dir=$(mktemp -d)
    cleanup() {
      rm -rf "$tmp_dir"
    }
    trap cleanup EXIT INT HUP TERM
    archive_path="$tmp_dir/$asset"
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "$url" -o "$archive_path"
    else
      wget -qO "$archive_path" "$url"
    fi
    tar -xzf "$archive_path" -C "$LOCAL_BIN_DIR" search-paper-cli
    chmod +x "$LOCAL_BIN_PATH"
    printf '%s\n' "$LOCAL_BIN_PATH"
    exit 0
  fi
fi

printf 'error: search-paper-cli is missing and automatic installation is unavailable.\n' >&2
printf 'error: install Go and rerun this script, or install the CLI manually from https://github.com/jtsang4/search-paper-cli.\n' >&2
exit 1
