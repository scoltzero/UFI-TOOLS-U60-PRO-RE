#!/bin/sh
# Build a local, checksum-covered script bundle. No signing keys or upload.
set -eu
PROJECT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
VERSION=${VERSION:-0.0.1}
OUT=${1:-$PROJECT/build/script-bundle-$VERSION}
case "$OUT" in /*) ;; *) OUT=$PWD/$OUT ;; esac
[ ! -e "$OUT" ] || { echo "output already exists: $OUT" >&2; exit 1; }
mkdir -p "$(dirname "$OUT")"
stage=$(mktemp -d "$(dirname "$OUT")/.ufi-package.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
cd "$PROJECT"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o "$stage/ufi-tools-u60pro" ./cmd/ufi-tools-u60pro
cp installer/bundle/service.sh "$stage/service.sh"
cp installer/install-adb.sh "$stage/install-adb.sh"
cp install.sh install.command LICENSE NOTICE README.md "$stage/"
printf 'version=%s\ncommit=%s\n' "$VERSION" "$(git rev-parse HEAD)" > "$stage/build-info.txt"
git diff --quiet || printf 'worktree=modified\n' >> "$stage/build-info.txt"
chmod 755 "$stage/install.sh" "$stage/install.command" "$stage/install-adb.sh" "$stage/service.sh" "$stage/ufi-tools-u60pro"
(
  cd "$stage"
  for name in LICENSE NOTICE README.md build-info.txt install.sh install.command install-adb.sh service.sh ufi-tools-u60pro; do
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$name"; else shasum -a 256 "$name"; fi
  done > SHA256SUMS
)
mv "$stage" "$OUT"
echo "local script bundle: $OUT"
echo "install: $OUT/install.sh"
echo "preview: $OUT/install.sh preview"
