#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

DIST_DIR="$REPO_ROOT/dist"
TARGETS="
linux amd64
linux arm64
darwin amd64
darwin arm64
windows amd64
"

cd "$REPO_ROOT"

GOMAXPROCS=8 go test -run '^$' -p 8 ./...
test -z "$(gofmt -l .)"
GOMAXPROCS=8 go test -count=1 -p 8 ./...
GOMAXPROCS=8 go build ./...

printf '%s' "$TARGETS" | while IFS=' ' read -r target_os target_arch; do
  [ -n "${target_os}" ] || continue

  artifact_name="search-paper-cli_${target_os}_${target_arch}"
  artifact_dir="$DIST_DIR/$artifact_name"
  binary_name="search-paper-cli"
  archive_path="$DIST_DIR/${artifact_name}.tar.gz"

  if [ "$target_os" = "windows" ]; then
    binary_name="${binary_name}.exe"
    archive_path="$DIST_DIR/${artifact_name}.zip"
  fi

  binary_path="$artifact_dir/$binary_name"

  mkdir -p "$artifact_dir"
  rm -f "$binary_path" "$archive_path"

  GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 go build -o "$binary_path" ./cmd/search-paper-cli

  if [ "$target_os" = "windows" ]; then
    python3 - <<'PY' "$artifact_dir" "$binary_name" "$archive_path"
import pathlib
import sys
import zipfile

artifact_dir = pathlib.Path(sys.argv[1])
binary_name = sys.argv[2]
archive_path = pathlib.Path(sys.argv[3])

with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
    archive.write(artifact_dir / binary_name, arcname=binary_name)
PY
  else
    tar -czf "$archive_path" -C "$artifact_dir" "$binary_name"
  fi

  printf '%s\n' "$binary_path"
  printf '%s\n' "$archive_path"
done
