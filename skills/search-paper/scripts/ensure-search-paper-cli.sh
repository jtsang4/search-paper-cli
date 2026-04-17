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
5. On supported platforms, download the latest GitHub release artifact into ./bin/
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
MODULE_PATH="github.com/jtsang4/search-paper-cli/cmd/search-paper-cli"
os=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')
arch=$(uname -m 2>/dev/null)
binary_name="search-paper-cli"

case "$os" in
  linux)
    os="linux"
    ;;
  darwin)
    os="darwin"
    ;;
  mingw*|msys*|cygwin*)
    os="windows"
    binary_name="search-paper-cli.exe"
    ;;
esac

case "$arch" in
  x86_64)
    arch="amd64"
    ;;
  amd64)
    arch="amd64"
    ;;
  arm64|aarch64)
    arch="arm64"
    ;;
esac

LOCAL_BIN_PATH="$LOCAL_BIN_DIR/$binary_name"

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

if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
  case "$os:$arch" in
    linux:amd64|linux:arm64|darwin:amd64|darwin:arm64)
      if command -v tar >/dev/null 2>&1; then
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
        tar -xzf "$archive_path" -C "$LOCAL_BIN_DIR" "$binary_name"
        chmod +x "$LOCAL_BIN_PATH"
        printf '%s\n' "$LOCAL_BIN_PATH"
        exit 0
      fi
      ;;
    windows:amd64)
      if command -v python3 >/dev/null 2>&1; then
        asset="search-paper-cli_${os}_${arch}.zip"
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
        python3 - <<'PY' "$archive_path" "$LOCAL_BIN_DIR" "$binary_name"
import pathlib
import sys
import zipfile

archive_path = pathlib.Path(sys.argv[1])
target_dir = pathlib.Path(sys.argv[2])
binary_name = sys.argv[3]

with zipfile.ZipFile(archive_path) as archive:
    archive.extract(binary_name, path=target_dir)
PY
        printf '%s\n' "$LOCAL_BIN_PATH"
        exit 0
      fi
      ;;
  esac
fi

printf 'error: search-paper-cli is missing and automatic installation is unavailable.\n' >&2
printf 'error: install Go and rerun this script, or install the CLI manually from https://github.com/jtsang4/search-paper-cli.\n' >&2
exit 1
