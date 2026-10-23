#!/bin/sh
# CLI only: backup explicitly contacts the router; prepare and dry-run are offline.
set -eu
PROJECT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
if [ -x "$SCRIPT_DIR/ufi-lan-installer" ]; then DEFAULT_BIN=$SCRIPT_DIR/ufi-lan-installer; else DEFAULT_BIN=$PROJECT/bin/ufi-lan-installer; fi
BIN=${UFI_LAN_INSTALLER_BIN:-$DEFAULT_BIN}
if [ ! -x "$BIN" ]; then
  (cd "$PROJECT" && go build -buildvcs=false -trimpath -o "$BIN" ./cmd/lan-installer)
fi
exec "$BIN" "$@"
