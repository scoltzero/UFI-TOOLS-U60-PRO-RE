#!/bin/sh
set -eu

MODE=dry-run
ACTION=install
ADB=${ADB:-adb}
SERIAL=
BUNDLE=
TARGET_ROOT=/data/ufi-tools-re
TARGET_PORT=2433
TTYD_PORT=1147
STORE_PORT=2341
AUTOSTART=1
SAFE_MODE=1
CONFIRM=0
MIN_FREE_MB=64
UNINSTALL_MODE=keep-backup
UNINSTALL_MODE_SET=0
STAGING=

usage() {
  echo "usage: $0 [--execute] [--serial SERIAL] --bundle DIRECTORY"
  echo "       [--root DIR] [--port PORT] [--ttyd-port PORT] [--store-port PORT]"
  echo "       [--no-autostart] [--safe-mode] [--min-free-mb MB] [--staging DIR]"
  echo "       $0 [--execute] [--serial SERIAL] --uninstall [--root DIR] [--no-autostart]"
  echo "       [--keep-data|--keep-backup|--purge|--restore] [--yes]"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --execute) MODE=execute ;;
    --uninstall) ACTION=uninstall ;;
    --serial) shift; SERIAL=${1:-} ;;
    --bundle) shift; BUNDLE=${1:-} ;;
    --root) shift; TARGET_ROOT=${1:-} ;;
    --port) shift; TARGET_PORT=${1:-} ;;
    --ttyd-port) shift; TTYD_PORT=${1:-} ;;
    --store-port) shift; STORE_PORT=${1:-} ;;
    --no-autostart) AUTOSTART=0 ;;
    --safe-mode) SAFE_MODE=1 ;;
    --yes) CONFIRM=1 ;;
    --min-free-mb) shift; MIN_FREE_MB=${1:-} ;;
    --keep-data) UNINSTALL_MODE=keep-data; UNINSTALL_MODE_SET=1 ;;
    --keep-backup) UNINSTALL_MODE=keep-backup; UNINSTALL_MODE_SET=1 ;;
    --purge) UNINSTALL_MODE=purge; UNINSTALL_MODE_SET=1 ;;
    --restore) UNINSTALL_MODE=restore; UNINSTALL_MODE_SET=1 ;;
    --staging) shift; STAGING=${1:-} ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

require_port() {
  case "$2" in
    ''|*[!0-9]*) echo "$1 must be a number: $2" >&2; exit 2 ;;
  esac
  if [ "$2" -lt 1 ] || [ "$2" -gt 65535 ]; then
    echo "$1 must be between 1 and 65535: $2" >&2
    exit 2
  fi
}

require_remote_path() {
  case "$2" in
    /data/ufi-tools|/data/ufi-tools/*|/data/ufi-tools.*|*//*|*/./*|*/.|*/)
      echo "$1 refers to the original tree or a non-canonical path" >&2; exit 2 ;;
    /|/data|/data/|/data/local/tmp|/data/local/tmp/|/tmp|/tmp/|/etc|/etc/|/sdcard|/sdcard/|/system|/system/|/usr|/usr/)
      echo "$1 is unsafe: $2" >&2
      exit 2
      ;;
    *..*|*[!a-zA-Z0-9_./-]*)
      echo "$1 contains unsafe characters: $2" >&2
      exit 2
      ;;
    /*) ;;
    *)
      echo "$1 must be absolute: $2" >&2
      exit 2
      ;;
  esac
}

require_port "--port" "$TARGET_PORT"
require_port "--ttyd-port" "$TTYD_PORT"
require_port "--store-port" "$STORE_PORT"
require_remote_path "--root" "$TARGET_ROOT"
case "$TARGET_ROOT" in /data/ufi-tools|/data/ufi-tools/*|/data/ufi-tools.*)
  echo "refusing to modify the original UFI-TOOLS installation" >&2; exit 2 ;; esac
case "$MIN_FREE_MB" in
  ''|*[!0-9]*) echo "--min-free-mb must be a number: $MIN_FREE_MB" >&2; exit 2 ;;
esac
[ "$MIN_FREE_MB" -ge 1 ] || { echo "--min-free-mb must be at least 1" >&2; exit 2; }
if [ "$ACTION" = install ] && [ "$UNINSTALL_MODE_SET" = 1 ]; then
  echo "uninstall modes cannot be used with an install" >&2
  exit 2
fi

if [ -z "$STAGING" ]; then STAGING=${TARGET_ROOT}.staging; fi
require_remote_path "--staging" "$STAGING"
[ "$TARGET_ROOT" != "$STAGING" ] || { echo "--root and --staging must differ" >&2; exit 2; }
case "$STAGING/" in
  "$TARGET_ROOT/"*|"$TARGET_ROOT.previous/"*|"$TARGET_ROOT.next/"*|"$TARGET_ROOT.uninstalled/"*|"$TARGET_ROOT.restore-reserve/"*)
    echo "staging must be outside the installation and recovery trees" >&2; exit 2 ;;
esac
case "$TARGET_ROOT/" in
  "$STAGING/"*) echo "staging must not contain the install root" >&2; exit 2 ;;
esac

if [ "$ACTION" = install ]; then
  [ -n "$BUNDLE" ] || { usage >&2; exit 2; }
  for required in ufi-tools-u60pro service.sh SHA256SUMS; do
    [ -f "$BUNDLE/$required" ] || { echo "missing $required" >&2; exit 2; }
  done
fi

RC_LOCAL=/etc/rc.local
RC_LINE="sh $TARGET_ROOT/service.sh start"
# Uninstall loads saved ports and boot settings, rather than resetting them.
REMOTE_ENV="UFI_INSTALL_ROOT='$TARGET_ROOT' UFI_STAGING_DIR='$STAGING'"
if [ "$ACTION" = install ]; then
  REMOTE_ENV="$REMOTE_ENV UFI_HTTP_ADDR=':$TARGET_PORT' UFI_TTYD_PORT='$TTYD_PORT' UFI_PLUGIN_STORE_ADDR='127.0.0.1:$STORE_PORT' UFI_RC_LOCAL='$RC_LOCAL' UFI_AUTOSTART='$AUTOSTART'"
fi
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
if [ -f "$SCRIPT_DIR/bundle/service.sh" ]; then CONTROLLER=$SCRIPT_DIR/bundle/service.sh; else CONTROLLER=$SCRIPT_DIR/service.sh; fi
[ -f "$CONTROLLER" ] || { echo "missing device controller: $CONTROLLER" >&2; exit 2; }

adb_raw() {
  if [ -n "$SERIAL" ]; then
    "$ADB" -s "$SERIAL" "$@"
  else
    "$ADB" "$@"
  fi
}

adb_target() {
  if [ "${1:-}" != shell ]; then adb_raw "$@"; return; fi
  shift
  # Older adbd versions return host status 0 even for a failed remote shell.
  # Run the payload in its own shell so an explicit exit still emits a result.
  adb_output=$(mktemp "${TMPDIR:-/tmp}/ufi-adb-output.XXXXXX") || return 1
  adb_marker=__UFI_ADB_EXIT_$$
  adb_command=$(printf '%s' "$*" | sed "s/'/'\\\\''/g")
  adb_host_status=0
  adb_raw shell "sh -c '$adb_command'; adb_remote_status=\$?; printf '\n$adb_marker=%s\n' \"\$adb_remote_status\"" > "$adb_output" || adb_host_status=$?
  adb_last=$(tail -n 1 "$adb_output" | tr -d '\r')
  case "$adb_last" in
    "$adb_marker="*)
      adb_status=${adb_last#*=}
      case "$adb_status" in ''|*[!0-9]*) adb_status=256 ;; esac
      if [ "$adb_status" -le 255 ]; then
        sed '$d' "$adb_output"
        rm -f "$adb_output"
        [ "$adb_host_status" -eq 0 ] || return "$adb_host_status"
        return "$adb_status"
      fi
      ;;
  esac
  cat "$adb_output"
  rm -f "$adb_output"
  echo 'ADB shell did not return a valid remote exit status' >&2
  [ "$adb_host_status" -ne 0 ] || adb_host_status=1
  return "$adb_host_status"
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

require_remote_root() {
  uid=$(adb_target shell id -u 2>/dev/null | tr -d '\r')
  case "$uid" in
    0) return 0 ;;
  esac
  id_output=$(adb_target shell id 2>/dev/null | tr -d '\r')
  case "$id_output" in
    *"uid=0("*) return 0 ;;
  esac
  echo "ADB shell must run as root (id -u=$uid)" >&2
  exit 1
}

preflight_install() {
  target_parent=$(dirname "$TARGET_ROOT")
  require_remote_root
  if ! adb_target shell "[ -w '$target_parent' ]"; then
    echo "target directory is not writable: $target_parent" >&2
    exit 1
  fi
  if adb_target shell "[ -e '$TARGET_ROOT' ]"; then
    if ! adb_target shell "grep -Fqx '# ufi-tools managed installation' '$TARGET_ROOT/.ufi-tools-manifest'"; then
      echo "refusing to install over unowned directory: $TARGET_ROOT" >&2
      exit 1
    fi
  fi
  free_kb=$(adb_target shell "df -Pk '$target_parent' 2>/dev/null | awk 'NR==2 {print \$4}'" | tr -d '\r')
  case "$free_kb" in
    ''|*[!0-9]*)
      echo "cannot determine free space on $target_parent" >&2; exit 1
      ;;
    *)
      bundle_kb=$(du -sk "$BUNDLE" | awk '{print $1}')
      active_kb=$(adb_target shell "if [ -d '$TARGET_ROOT' ]; then du -sk '$TARGET_ROOT' | awk '{print \$1}'; else echo 0; fi" | tr -d '\r')
      case "$active_kb" in ''|*[!0-9]*) echo "cannot determine active tree size" >&2; exit 1 ;; esac
      need_kb=$((2 * bundle_kb + active_kb + 16384))
      floor_kb=$((MIN_FREE_MB * 1024))
      [ "$need_kb" -ge "$floor_kb" ] || need_kb=$floor_kb
      if [ "$free_kb" -lt "$need_kb" ]; then
        echo "insufficient free space on $target_parent: ${free_kb} KiB available, ${need_kb} KiB required" >&2
        exit 1
      fi
      ;;
  esac
}

preflight_uninstall() {
  require_remote_root
  # The current controller validates ownership; no active service is required
  # for a second uninstall or recovery of a saved installation.
}

prepare_staging() {
  adb_target shell "set -eu; umask 077; [ ! -L '$STAGING' ] && [ ! -L '$STAGING.owner' ] && [ ! -L '$STAGING.controller' ] || exit 1; if [ -e '$STAGING' ] || [ -e '$STAGING.controller' ]; then [ -f '$STAGING.owner' ] && [ \"\$(cat '$STAGING.owner')\" = '$TARGET_ROOT' ] || exit 1; rm -rf '$STAGING'; fi; mkdir -p '$STAGING'; printf '%s\\n' '$TARGET_ROOT' > '$STAGING.owner'"
  adb_target push "$CONTROLLER" "$STAGING.controller" >/dev/null
}

cleanup_staging() {
  adb_target shell "if [ -f '$STAGING.owner' ] && [ \"\$(cat '$STAGING.owner')\" = '$TARGET_ROOT' ]; then rm -rf '$STAGING'; rm -f '$STAGING.owner' '$STAGING.controller'; fi"
}

echo "transport: authorized ADB"
echo "target: $TARGET_ROOT"
echo "http port: $TARGET_PORT"
echo "autostart: $([ "$AUTOSTART" = 1 ] && echo enabled || echo disabled)"
echo "mode: $([ "$SAFE_MODE" = 1 ] && echo safe || echo normal)"
echo "staging: $STAGING"
if [ "$ACTION" = uninstall ]; then
  echo "uninstall mode: $UNINSTALL_MODE"
  echo "transaction: stop -> remove autostart entry -> remove own files"
else
  echo "bundle: $BUNDLE"
  echo "transaction: verify -> stage -> stop -> migrate -> rotate -> start -> health-check -> autostart -> rollback"
fi

if [ "$MODE" != execute ]; then
  echo "dry-run only; pass --execute to contact and modify the selected device"
  exit 0
fi

command -v "$ADB" >/dev/null 2>&1 || { echo "adb not found" >&2; exit 1; }

require_authorized_adb

if [ "$ACTION" = uninstall ]; then
  preflight_uninstall
  prepare_staging
  adb_target shell "$REMOTE_ENV sh '$STAGING.controller' uninstall --$UNINSTALL_MODE"
  cleanup_staging
  exit 0
fi

ARCH=$(adb_target shell uname -m | tr -d '\r')
[ "$ARCH" = aarch64 ] || [ "$ARCH" = arm64 ] || { echo "unsupported architecture: $ARCH" >&2; exit 1; }
preflight_install
prepare_staging
# Upload the complete manifest-covered bundle; the controller is separate and
# verifies every payload file before invoking any shipped program.
adb_target push "$BUNDLE/." "$STAGING/" >/dev/null
adb_target shell "$REMOTE_ENV sh '$STAGING.controller' install '$STAGING'"
cleanup_staging
