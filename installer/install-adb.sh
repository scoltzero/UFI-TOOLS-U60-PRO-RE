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

adb_target() {
  if [ -n "$SERIAL" ]; then
    "$ADB" -s "$SERIAL" "$@"
  else
    "$ADB" "$@"
  fi
}

detect_adb_state() {
  state=$(adb_target get-state 2>/dev/null || true)
  [ -n "$state" ] && { printf '%s\n' "$state"; return; }

  if [ -n "$SERIAL" ]; then
    "$ADB" devices 2>/dev/null | awk -v serial="$SERIAL" '$1 == serial { print $2; exit }'
    return
  fi

  "$ADB" devices 2>/dev/null | awk '
    NR > 1 && NF >= 2 { count++; state=$2 }
    END {
      if (count == 1) print state
      else if (count > 1) print "multiple"
    }
  '
}

require_authorized_adb() {
  state=$(detect_adb_state)
  case "$state" in
    device) return ;;
    unauthorized)
      echo "ADB device is connected but not authorized; approve the USB debugging prompt on the device" >&2
      ;;
    offline)
      echo "ADB device is offline; disable and re-enable USB/network debugging, then reconnect" >&2
      ;;
    multiple)
      echo "multiple ADB devices are connected; select the U60 Pro with --serial SERIAL" >&2
      ;;
    '')
      echo "no ADB interface detected; verify the data cable and enable USB debugging on the U60 Pro" >&2
      ;;
    *)
      echo "ADB device is not ready (state: $state)" >&2
      ;;
  esac
  exit 1
}

echo "transport: authorized ADB"
echo "bundle: $BUNDLE"
echo "target: /data/ufi-tools"
echo "transaction: verify -> stage -> rotate -> start -> health-check -> rollback"

if [ "$MODE" != execute ]; then
  echo "dry-run only; pass --execute to contact and modify the selected device"
  exit 0
fi

command -v "$ADB" >/dev/null 2>&1 || { echo "adb not found" >&2; exit 1; }

require_authorized_adb
ARCH=$(adb_target shell uname -m | tr -d '\r')
[ "$ARCH" = aarch64 ] || [ "$ARCH" = arm64 ] || { echo "unsupported architecture: $ARCH" >&2; exit 1; }

REMOTE=/data/local/tmp/ufi-tools-u60pro-install
adb_target shell "rm -rf '$REMOTE' && mkdir -p '$REMOTE'"
for file in manifest.json ufi-tools-u60pro verify-release public-key.hex service.sh; do
  adb_target push "$BUNDLE/$file" "$REMOTE/$file" >/dev/null
done
for file in ttyd speedtest sendat ufi-plugin-store; do
  if [ -f "$BUNDLE/$file" ]; then
    adb_target push "$BUNDLE/$file" "$REMOTE/$file" >/dev/null
  fi
done
if [ -d "$BUNDLE/plugin-store" ]; then
  adb_target push "$BUNDLE/plugin-store" "$REMOTE/plugin-store" >/dev/null
fi
adb_target shell "chmod 0755 '$REMOTE/ufi-tools-u60pro' '$REMOTE/verify-release' '$REMOTE/service.sh' '$REMOTE/ttyd' '$REMOTE/speedtest' 2>/dev/null || true; '$REMOTE/service.sh' install '$REMOTE'"
