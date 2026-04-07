#!/usr/bin/env sh
set -eu

REPO_ROOT="/root/workspace/github/jtsang4/search-paper-cli"

mkdir -p "$REPO_ROOT/dist" "$REPO_ROOT/.tmp"

if [ -f "$REPO_ROOT/go.mod" ]; then
  cd "$REPO_ROOT"
  go mod download
fi
