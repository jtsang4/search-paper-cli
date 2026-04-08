#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

DIST_DIR="$REPO_ROOT/dist"
ARTIFACT_DIR="$DIST_DIR/search-paper-cli_linux_amd64"
BINARY_PATH="$ARTIFACT_DIR/search-paper-cli"
ARCHIVE_PATH="$DIST_DIR/search-paper-cli_linux_amd64.tar.gz"

cd "$REPO_ROOT"

GOMAXPROCS=8 go test -run '^$' -p 8 ./...
test -z "$(gofmt -l .)"
GOMAXPROCS=8 go test -count=1 -p 8 ./...
GOMAXPROCS=8 go build ./...

mkdir -p "$ARTIFACT_DIR"
rm -f "$BINARY_PATH" "$ARCHIVE_PATH"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BINARY_PATH" ./cmd/search-paper-cli
tar -czf "$ARCHIVE_PATH" -C "$ARTIFACT_DIR" search-paper-cli

printf '%s\n' "$BINARY_PATH"
printf '%s\n' "$ARCHIVE_PATH"
