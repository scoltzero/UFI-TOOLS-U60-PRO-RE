#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
LOCK=$ROOT/UPSTREAM.lock
DEST=${1:-$ROOT/third_party/UFI-TOOLS}

commit=$(sed -n 's/^commit=//p' "$LOCK")
[ -n "$commit" ] || { echo "missing upstream commit" >&2; exit 1; }
[ ! -e "$DEST" ] || { echo "destination already exists: $DEST" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
archive=$tmp/upstream.tar.gz
curl -fL --proto '=https' --tlsv1.2 \
  "https://github.com/kanoqwq/UFI-TOOLS/archive/$commit.tar.gz" \
  -o "$archive"
mkdir -p "$DEST"
tar -xzf "$archive" --strip-components=1 -C "$DEST"
echo "fetched upstream UFI-TOOLS $commit into $DEST"

