#!/bin/sh
set -eu

MODE=dry-run
ADB=${ADB:-adb}
SERIAL=
BUNDLE=

usage() {
  echo "usage: $0 [--execute] [--serial SERIAL] --bundle DIRECTORY"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --execute) MODE=execute ;;
    --serial) shift; SERIAL=${1:-} ;;
    --bundle) shift; BUNDLE=${1:-} ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

[ -n "$BUNDLE" ] || { usage >&2; exit 2; }
[ -f "$BUNDLE/manifest.json" ] || { echo "missing manifest.json" >&2; exit 2; }
[ -f "$BUNDLE/ufi-tools-u60pro" ] || { echo "missing service binary" >&2; exit 2; }
[ -f "$BUNDLE/verify-release" ] || { echo "missing release verifier" >&2; exit 2; }
[ -f "$BUNDLE/public-key.hex" ] || { echo "missing trusted public key" >&2; exit 2; }
[ -f "$BUNDLE/service.sh" ] || { echo "missing service.sh" >&2; exit 2; }

ADB_TARGET=$ADB
[ -z "$SERIAL" ] || ADB_TARGET="$ADB -s $SERIAL"

echo "transport: authorized ADB"
echo "bundle: $BUNDLE"
echo "target: /data/ufi-tools"
echo "transaction: verify -> stage -> rotate -> start -> health-check -> rollback"

if [ "$MODE" != execute ]; then
  echo "dry-run only; pass --execute to contact and modify the selected device"
  exit 0
fi

command -v "$ADB" >/dev/null 2>&1 || { echo "adb not found" >&2; exit 1; }

# shellcheck disable=SC2086
$ADB_TARGET get-state >/dev/null
# shellcheck disable=SC2086
ARCH=$($ADB_TARGET shell uname -m | tr -d '\r')
[ "$ARCH" = aarch64 ] || [ "$ARCH" = arm64 ] || { echo "unsupported architecture: $ARCH" >&2; exit 1; }

REMOTE=/data/local/tmp/ufi-tools-u60pro-install
# shellcheck disable=SC2086
$ADB_TARGET shell "rm -rf '$REMOTE' && mkdir -p '$REMOTE'"
for file in manifest.json ufi-tools-u60pro verify-release public-key.hex service.sh; do
  # shellcheck disable=SC2086
  $ADB_TARGET push "$BUNDLE/$file" "$REMOTE/$file" >/dev/null
done
for file in ttyd speedtest sendat ufi-plugin-store; do
  if [ -f "$BUNDLE/$file" ]; then
    # shellcheck disable=SC2086
    $ADB_TARGET push "$BUNDLE/$file" "$REMOTE/$file" >/dev/null
  fi
done
if [ -d "$BUNDLE/plugin-store" ]; then
  # shellcheck disable=SC2086
  $ADB_TARGET push "$BUNDLE/plugin-store" "$REMOTE/plugin-store" >/dev/null
fi
# shellcheck disable=SC2086
$ADB_TARGET shell "chmod 0755 '$REMOTE/ufi-tools-u60pro' '$REMOTE/verify-release' '$REMOTE/service.sh' '$REMOTE/ttyd' '$REMOTE/speedtest' 2>/dev/null || true; '$REMOTE/service.sh' install '$REMOTE'"
