#!/bin/sh
set -eu

OUT=${1:?output directory required}
rm -rf "$OUT"
mkdir -p "$OUT"
INDEX=$OUT/modules.tsv
: > "$INDEX"

set -- $(go list -e ./... | grep -v '/third_party/')
[ "$#" -gt 0 ] || { echo "no project packages found" >&2; exit 1; }

GOFLAGS=-buildvcs=false go list -deps \
  -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' "$@" |
  sort -u |
  while IFS='|' read -r module version directory; do
    [ -n "$module" ] || continue
    safe=$(printf '%s' "$module" | tr '/.' '__')
    license=$(find "$directory" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) -print -quit)
    [ -n "$license" ] || { echo "missing license for $module" >&2; exit 1; }
    target=${safe}-${version}-$(basename "$license")
    cp "$license" "$OUT/$target"
    printf '%s\t%s\t%s\n' "$module" "$version" "$target" >> "$INDEX"
  done
