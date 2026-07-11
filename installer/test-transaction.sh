#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TMP=${TMPDIR:-/tmp}/ufi-tools-install-test-$$
ROOT=$TMP/root
BUNDLE=$TMP/bundle
TOOLS=$TMP/tools
mkdir -p "$ROOT/data" "$BUNDLE" "$TOOLS"
trap 'UFI_INSTALL_ROOT="$ROOT" "$PROJECT_ROOT/installer/bundle/service.sh" stop >/dev/null 2>&1 || true; rm -rf "$TMP"' EXIT INT TERM

echo old > "$ROOT/version-marker"
printf '%s\n' 'optional-tool' > "$ROOT/ttyd"
cp "$PROJECT_ROOT/installer/bundle/service.sh" "$BUNDLE/service.sh"
printf '%s\n' '#!/bin/sh' 'while :; do sleep 60; done' > "$BUNDLE/ufi-tools-u60pro"
printf '%s\n' '#!/bin/sh' 'exit 0' > "$BUNDLE/verify-release"
printf '%s\n' '{}' > "$BUNDLE/manifest.json"
printf '%s\n' 'test-key' > "$BUNDLE/public-key.hex"
printf '%s\n' '#!/bin/sh' 'exit "${FAKE_WGET_STATUS:-0}"' > "$TOOLS/wget"
chmod 0755 "$BUNDLE/ufi-tools-u60pro" "$BUNDLE/verify-release" "$BUNDLE/service.sh" "$TOOLS/wget"

PATH="$TOOLS:$PATH" UFI_INSTALL_ROOT="$ROOT" UFI_HEALTH_ATTEMPTS=1 "$BUNDLE/service.sh" install "$BUNDLE"
[ -x "$ROOT/ufi-tools-u60pro" ]
[ -d "$ROOT/data" ]
[ "$(cat "$ROOT/ttyd")" = optional-tool ]
PATH="$TOOLS:$PATH" UFI_INSTALL_ROOT="$ROOT" "$ROOT/service.sh" stop

echo rollback-source > "$ROOT/version-marker"
if PATH="$TOOLS:$PATH" FAKE_WGET_STATUS=1 UFI_INSTALL_ROOT="$ROOT" UFI_HEALTH_ATTEMPTS=1 "$BUNDLE/service.sh" install "$BUNDLE"; then
  echo "expected failed health check" >&2
  exit 1
fi
[ "$(cat "$ROOT/version-marker")" = rollback-source ]
PATH="$TOOLS:$PATH" UFI_INSTALL_ROOT="$ROOT" "$ROOT/service.sh" stop

echo "transaction and rollback tests passed"
