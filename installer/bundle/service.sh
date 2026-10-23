#!/bin/sh
set -eu

SCRIPT_HOME=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
ROOT=${UFI_INSTALL_ROOT:-/data/ufi-tools-re}
if [ -z "${UFI_INSTALL_ROOT:-}" ] && [ -f "$SCRIPT_HOME/.ufi-tools-manifest" ]; then ROOT=$SCRIPT_HOME; fi
# Plain data, never sourced/evaluated. Explicit environment overrides win.
runtime=$ROOT/.ufi-runtime
if [ ! -d "$ROOT" ]; then
  for old in "$ROOT.previous" "$ROOT.uninstalled"; do
    if [ -f "$old/.ufi-runtime" ]; then runtime=$old/.ufi-runtime; break; fi
  done
fi
if [ -f "$runtime" ]; then
  while IFS='=' read -r key value; do
    case "$key" in
      HTTP_ADDR) UFI_HTTP_ADDR=${UFI_HTTP_ADDR-$value} ;;
      TTYD_PORT) UFI_TTYD_PORT=${UFI_TTYD_PORT-$value} ;;
      STORE_ADDR) UFI_PLUGIN_STORE_ADDR=${UFI_PLUGIN_STORE_ADDR-$value} ;;
      AUTOSTART) UFI_AUTOSTART=${UFI_AUTOSTART-$value} ;;
      RC_LOCAL) UFI_RC_LOCAL=${UFI_RC_LOCAL-$value} ;;
      DEVICE_BACKEND) UFI_DEVICE_BACKEND=${UFI_DEVICE_BACKEND-$value} ;;
      DEVICE_BASE_URL) UFI_DEVICE_BASE_URL=${UFI_DEVICE_BASE_URL-$value} ;;
      WORKERS_ENABLED) UFI_WORKERS_ENABLED=${UFI_WORKERS_ENABLED-$value} ;;
      PRIVILEGED_COMPAT) UFI_PRIVILEGED_COMPAT=${UFI_PRIVILEGED_COMPAT-$value} ;;
      AT_ENABLED) UFI_AT_ENABLED=${UFI_AT_ENABLED-$value} ;;
    esac
  done < "$runtime"
fi
UFI_DEVICE_BACKEND=${UFI_DEVICE_BACKEND:-u60-local}
if [ "$UFI_DEVICE_BACKEND" = u60-ubus ] || [ "$UFI_DEVICE_BACKEND" = zte-http ]; then
  UFI_DEVICE_BASE_URL=${UFI_DEVICE_BASE_URL-http://127.0.0.1}
else
  UFI_DEVICE_BASE_URL=${UFI_DEVICE_BASE_URL-}
fi
NEXT=${ROOT}.next
BACKUP=${ROOT}.previous
UNINSTALLED=${ROOT}.uninstalled
RESTORE_RESERVE=${ROOT}.restore-reserve
PIDFILE=$ROOT/ufi-tools.pid
PENDING=$ROOT/.ufi-tools-pending
LEGACY_PIDFILE=$ROOT/ufi-tools-u60pro.pid
STORE_PIDFILE=$ROOT/ufi-plugin-store.pid
TTYD_PIDFILE=$ROOT/data/ttyd.pid
HTTP_ADDR=${UFI_HTTP_ADDR:-:2433}
HTTP_PORT=${UFI_HTTP_PORT:-${HTTP_ADDR##*:}}
HEALTH_PORT=${UFI_HEALTH_PORT:-$HTTP_PORT}
HEALTH_URL=${UFI_HEALTH_URL:-http://127.0.0.1:$HEALTH_PORT/healthz}
HEALTH_ATTEMPTS=${UFI_HEALTH_ATTEMPTS:-20}
HEALTH_SETTLE_SECONDS=${UFI_HEALTH_SETTLE_SECONDS:-1}
STOP_ATTEMPTS=${UFI_STOP_ATTEMPTS:-10}
HOSTS_PATH=
PLUGIN_STORE_ADDR=${UFI_PLUGIN_STORE_ADDR:-127.0.0.1:2341}
TTYD_PORT=${UFI_TTYD_PORT:-1147}
AUTOSTART=${UFI_AUTOSTART:-1}
PLUGIN_BOOT_MARK=${UFI_PLUGIN_BOOT_MARK:-/tmp/.ufi_plugin_boot_done}
LEGACY_PLUGIN_BOOT_MARK=${UFI_LEGACY_PLUGIN_BOOT_MARK:-/tmp/.ufi-plugin-boot-done}
PLUGIN_MOUNT=${UFI_PLUGIN_MOUNT:-/sdcard}
PLUGIN_STORAGE=$ROOT/sdcard
PLUGIN_IMAGE=$ROOT/sdcard.img
MOUNTINFO=${UFI_MOUNTINFO:-/proc/self/mountinfo}
MOUNT_OWNER=${ROOT}.mount-owner
INSTANCE_FILE=$ROOT/.ufi-health-instance
RECOVERED=0
if [ -n "${UFI_RC_LOCAL:-}" ]; then
  RC_LOCAL=$UFI_RC_LOCAL
elif [ "$ROOT" = /data/ufi-tools-re ]; then
  RC_LOCAL=/etc/rc.local
else
  # Alternate install roots are test/staging trees; keep their boot entry
  # next to the tree instead of editing the host's real rc.local.
  RC_LOCAL=${ROOT}.rc.local
fi
RC_LINE="sh $ROOT/service.sh start"
RC_TX=${ROOT}.boot-transaction
if [ -n "${UFI_STAGING_DIR:-}" ]; then
  STAGING=$UFI_STAGING_DIR
elif [ "$ROOT" = /data/ufi-tools ]; then
  STAGING=/data/local/tmp/ufi-tools-re-install
else
  STAGING=${ROOT}.staging
fi
INVENTORY_NAME=.ufi-tools-manifest
INSTALL_ACTIVE=0
INSTALL_STOPPED=0
INSTALL_ROTATED=0
AUTOSTART_TOUCHED=0
install_status=0

pid_alive() {
  kill -0 "$1" 2>/dev/null || return 1
  if [ -r "/proc/$1/stat" ]; then
    state=$(awk '{print $3}' "/proc/$1/stat" 2>/dev/null || true)
    [ "$state" != Z ]
    return
  fi
  return 0
}

is_running() {
  [ -f "$1" ] || return 1
  pid=$(cat "$1" 2>/dev/null || true)
  case "$pid" in
    ''|*[!0-9]*) return 1 ;;
  esac
  pid_alive "$pid" && owned_pid "$pid"
}

is_mounted() {
  awk -v target="$1" '$5 == target { found=1 } END { exit !found }' "$MOUNTINFO" 2>/dev/null
}

release_plugin_storage() {
  # The isolated script never creates or detaches external mounts.
  check_tree_mounts
}

check_tree_mounts() {
  # A nested mount could make rm/cp traverse unrelated storage.
  if [ -r "$MOUNTINFO" ]; then
    if ! awk -v root="$ROOT" -v candidate="$NEXT" -v backup="$BACKUP" -v saved="$UNINSTALLED" -v reserve="$RESTORE_RESERVE" '
      $5 == root || index($5, root "/") == 1 ||
      $5 == candidate || index($5, candidate "/") == 1 ||
      $5 == backup || index($5, backup "/") == 1 ||
      $5 == saved || index($5, saved "/") == 1 ||
      $5 == reserve || index($5, reserve "/") == 1 { bad=1 }
      END { exit bad ? 1 : 0 }' "$MOUNTINFO"; then
      echo "refusing to change an installation containing mounted directories" >&2
      return 1
    fi
  fi
}

launch_detached() {
  launch_pidfile=$1
  launch_log=$2
  launch_binary=$3
  shift 3
  if command -v start-stop-daemon >/dev/null 2>&1; then
    # BusyBox daemonizes the wrapper; reopen log/stdin after its FD cleanup.
    # exec retains the PID written by -m, so ownership checks see the real binary.
    start-stop-daemon -S -b -m -p "$launch_pidfile" -x /bin/sh -- -c \
      'launch_log=$1; shift; exec "$@" >>"$launch_log" 2>&1 </dev/null' \
      ufi-detached "$launch_log" "$launch_binary" "$@"
  else
    nohup "$launch_binary" "$@" >>"$launch_log" 2>&1 </dev/null &
    echo $! > "$launch_pidfile"
  fi
}

start_store() {
  [ -x "$ROOT/ufi-plugin-store" ] || return 0
  [ -f "$ROOT/plugin-store/catalog.json" ] || return 0
  is_running "$STORE_PIDFILE" && return 0
  launch_detached "$STORE_PIDFILE" "$ROOT/plugin-store.log" "$ROOT/ufi-plugin-store" -root "$ROOT/plugin-store" -http-addr "$PLUGIN_STORE_ADDR"
}

start() {
  require_owned_root || return 1
  if [ "$INSTALL_ACTIVE" != 1 ]; then
    recover_interrupted || return 1
    if [ "$RECOVERED" = 1 ]; then
      start_restored
      return
    fi
  fi
  [ -x "$ROOT/ufi-tools-u60pro" ] || return 1
  is_running "$PIDFILE" && return 0
  start_store
  store_url=${UFI_PLUGIN_SERVER_URL:-}
  if [ -z "$store_url" ] && is_running "$STORE_PIDFILE"; then store_url=http://$PLUGIN_STORE_ADDR; fi
  instance=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
  [ "${#instance}" -eq 32 ] || return 1
  (umask 077; printf '%s\n' "$instance" > "$INSTANCE_FILE") || return 1
  deployment=$(cat "$ROOT/lan-plan-id" 2>/dev/null || true)
  if [ "${#deployment}" != 32 ]; then deployment=; fi
  case "$deployment" in *[!0-9a-f]*) deployment= ;; esac
  UFI_DATA_DIR="$ROOT/data" \
  UFI_DEVICE_TOKEN= \
  UFI_HEALTH_INSTANCE="$instance" \
  UFI_HEALTH_DEPLOYMENT="$deployment" \
  UFI_HTTP_ADDR="$HTTP_ADDR" \
  UFI_HOSTS_PATH="$HOSTS_PATH" \
  UFI_TTYD_BINARY="${UFI_TTYD_BINARY:-$ROOT/ttyd}" \
  UFI_TTYD_PORT="$TTYD_PORT" \
  UFI_SPEEDTEST_BINARY="${UFI_SPEEDTEST_BINARY:-$ROOT/speedtest}" \
  UFI_AT_BINARY="${UFI_AT_BINARY:-$ROOT/sendat}" \
  UFI_DEVICE_BACKEND="${UFI_DEVICE_BACKEND:-u60-local}" \
  UFI_DEVICE_BASE_URL="${UFI_DEVICE_BASE_URL-}" \
  UFI_WORKERS_ENABLED="${UFI_WORKERS_ENABLED:-false}" \
  UFI_PRIVILEGED_COMPAT="${UFI_PRIVILEGED_COMPAT:-false}" \
  UFI_AT_ENABLED="${UFI_AT_ENABLED:-false}" \
  UFI_PLUGIN_SERVER_URL="$store_url" \
  launch_detached "$PIDFILE" "$ROOT/service.log" "$ROOT/ufi-tools-u60pro"
}

add_stop_pid() {
  case "$1" in
    ''|*[!0-9]*) return 0 ;;
  esac
  case " $STOP_PIDS " in
    *" $1 "*) return 0 ;;
  esac
  STOP_PIDS="$STOP_PIDS $1"
}

collect_stop_pidfile() {
  [ -f "$1" ] || return 0
  pid=$(cat "$1" 2>/dev/null || true)
  rm -f "$1"
  owned_pid "$pid" && add_stop_pid "$pid"
  return 0
}

owned_pid() {
  case "$1" in ''|*[!0-9]*|0|1) return 1 ;; esac
  # Match executable arguments, not an arbitrary substring in a command line.
  if [ -r "/proc/$1/cmdline" ]; then
    tr '\000' '\n' < "/proc/$1/cmdline" | awk -v root="$ROOT" '
      NR==1 { first=$0 }
      (NR==1 || (NR==2 && first ~ /(^|\/)(sh|bash|dash|ash)$/)) &&
      ($0==root "/ufi-tools-u60pro" || $0==root "/ufi-plugin-store" || $0==root "/ttyd") { ok=1 }
      END { exit !ok }'
  else
    ps -p "$1" -o command= 2>/dev/null | awk -v root="$ROOT" '
      { target=$1; if ($1 ~ /(^|\/)(sh|bash|dash|ash)$/) target=$2 }
      target==root "/ufi-tools-u60pro" || target==root "/ufi-plugin-store" || target==root "/ttyd" { ok=1 }
      END { exit !ok }'
  fi
}

collect_stop_matches() {
  command -v pgrep >/dev/null 2>&1 || return 0
  for pid in $(pgrep -f "$1" 2>/dev/null || true); do
    owned_pid "$pid" && add_stop_pid "$pid"
  done
  return 0
}

wait_for_exit() {
  pid=$1
  i=0
  while pid_alive "$pid"; do
    if [ "$i" -ge "$STOP_ATTEMPTS" ]; then
      kill -KILL "$pid" 2>/dev/null || true
      sleep 1
      pid_alive "$pid" && return 1
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 0
}

stop() {
  # Stop the database writers and helpers, then wait for each PID to really
  # disappear before the caller copies or removes the active tree.
  STOP_PIDS=
  collect_stop_pidfile "$PIDFILE"
  collect_stop_pidfile "$LEGACY_PIDFILE"
  collect_stop_pidfile "$STORE_PIDFILE"
  collect_stop_pidfile "$TTYD_PIDFILE"
  collect_stop_matches "$ROOT/ufi-tools-u60pro"
  collect_stop_matches "$ROOT/ufi-plugin-store"
  collect_stop_matches "$ROOT/ttyd"
  for pid in $STOP_PIDS; do
    kill "$pid" 2>/dev/null || true
  done
  status=0
  for pid in $STOP_PIDS; do
    wait_for_exit "$pid" || status=1
  done
  return "$status"
}

health() {
  [ -r "$INSTANCE_FILE" ] || return 1
  expected_instance=$(cat "$INSTANCE_FILE")
  [ "${#expected_instance}" -eq 32 ] || return 1
  i=0
  while [ "$i" -lt "$HEALTH_ATTEMPTS" ]; do
    response=$(wget -q -T 3 -O - "$HEALTH_URL" 2>/dev/null) || response=
    if is_running "$PIDFILE" && printf '%s' "$response" | grep -Fq "\"instance\":\"$expected_instance\""; then
      if [ "$HEALTH_SETTLE_SECONDS" = 0 ]; then
        is_running "$PIDFILE" && return 0
      else
        sleep "$HEALTH_SETTLE_SECONDS"
        is_running "$PIDFILE" && return 0
      fi
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

rc_line_count() {
  [ -f "$RC_LOCAL" ] || { echo 0; return 0; }
  awk -v line="$RC_LINE" '$0 == line { n++ } END { print n + 0 }' "$RC_LOCAL" 2>/dev/null || echo 0
}

capture_rc() {
  dest=$1
  mkdir -p "$dest"
  chmod 700 "$dest" || return 1
  printf '%s\n' "$RC_LOCAL" > "$dest/path"
  if [ -L "$RC_LOCAL" ]; then readlink "$RC_LOCAL" > "$dest/link"; else : > "$dest/link"; fi
  if [ -e "$RC_LOCAL" ]; then
    [ -f "$RC_LOCAL" ] || return 1
    cp -p "$RC_LOCAL" "$dest/before" || return 1
  else
    [ ! -L "$RC_LOCAL" ] || return 1
    : > "$dest/absent"
  fi
  : > "$dest/complete"
}

write_rc_content() {
  source_content=$1
  target=$RC_LOCAL
  hops=0
  while [ -L "$target" ]; do
    hops=$((hops + 1))
    [ "$hops" -le 16 ] || return 1
    link=$(readlink "$target")
    case "$link" in /*) target=$link ;; *) target=$(dirname "$target")/$link ;; esac
  done
  parent=$(CDPATH='' cd -- "$(dirname "$target")" && pwd -P) || return 1
  target=$parent/$(basename "$target")
  if [ -e "$target" ]; then
    [ -f "$target" ] || return 1
    [ "$(ls -ld "$target" | awk '{print $2}')" = 1 ] || { echo "hard-linked boot files require manual handling" >&2; return 1; }
  fi
  replacement=$(mktemp "$parent/.ufi-rc.XXXXXX") || return 1
  if [ -e "$target" ]; then
    cp -p "$target" "$replacement" || { rm -f "$replacement"; return 1; }
  else
    chmod 755 "$replacement" || { rm -f "$replacement"; return 1; }
  fi
  # Failed writes only damage this temporary file, never the live boot file.
  cat "$source_content" > "$replacement" || { rm -f "$replacement"; return 1; }
  mv -f "$replacement" "$target" || { rm -f "$replacement"; return 1; }
}

restore_rc() {
  saved=$1
  [ -f "$saved/complete" ] || return 0
  [ "$(cat "$saved/path")" = "$RC_LOCAL" ] || { echo "boot file path differs from saved baseline" >&2; return 1; }
  [ -e "$RC_LOCAL" ] || return 0
  current_link=
  [ ! -L "$RC_LOCAL" ] || current_link=$(readlink "$RC_LOCAL")
  if [ "$current_link" != "$(cat "$saved/link")" ]; then
    echo "rc.local link changed since install; leaving it untouched" >&2
    return 1
  fi
  if [ -f "$saved/after" ] && cmp -s "$RC_LOCAL" "$saved/after"; then
    if [ -f "$saved/absent" ]; then rm -f "$RC_LOCAL"; else write_rc_content "$saved/before"; fi
  elif [ -f "$saved/added" ]; then
    [ "$(rc_line_count)" -gt 0 ] || return 0
    # Later user edits survive. Only the exact line we introduced is removed.
    tmp="$RC_LOCAL.ufi-tools.$$"
    awk -v line="$RC_LINE" '$0 != line' "$RC_LOCAL" > "$tmp" || return 1
    write_rc_content "$tmp" || { rm -f "$tmp"; return 1; }
    rm -f "$tmp"
  fi
}

enable_autostart() {
  require_safe_root && require_owned_root || return 1
  [ -d "$ROOT" ] || return 1
  if [ ! -f "$ROOT/.ufi-system/complete" ]; then capture_rc "$ROOT/.ufi-system" || return 1; fi
  [ "$(rc_line_count)" -eq 0 ] || return 0
  if [ -f "$RC_LOCAL" ]; then
    [ -w "$RC_LOCAL" ] || return 1
  else
    [ ! -L "$RC_LOCAL" ] || return 1
    [ -w "$(dirname "$RC_LOCAL")" ] || return 1
  fi
  tmp="$RC_LOCAL.ufi-tools.$$"
  if [ -f "$RC_LOCAL" ]; then
    awk -v line="$RC_LINE" '
    !added && /^[[:space:]]*exit([[:space:];]|$)/ { print line; added=1 }
    { print }
    END { if (!added) print line }' "$RC_LOCAL" > "$tmp" || return 1
  else
    printf '#!/bin/sh\n%s\n' "$RC_LINE" > "$tmp" || return 1
  fi
  AUTOSTART_TOUCHED=1
  : > "$ROOT/.ufi-system/added"
  [ ! -d "$RC_TX" ] || : > "$RC_TX/added"
  cp -p "$tmp" "$ROOT/.ufi-system/after" || return 1
  [ ! -d "$RC_TX" ] || cp -p "$tmp" "$RC_TX/after" || return 1
  write_rc_content "$tmp" || { rm -f "$tmp"; return 1; }
  rm -f "$tmp"
}

disable_autostart() {
  require_safe_root && require_owned_root || return 1
  restore_rc "$ROOT/.ufi-system"
}

write_runtime() {
  target=$1
  (umask 077
    printf 'HTTP_ADDR=%s\nTTYD_PORT=%s\nSTORE_ADDR=%s\nAUTOSTART=%s\nRC_LOCAL=%s\nDEVICE_BACKEND=%s\nDEVICE_BASE_URL=%s\nWORKERS_ENABLED=%s\nPRIVILEGED_COMPAT=%s\nAT_ENABLED=%s\n' \
      "$HTTP_ADDR" "$TTYD_PORT" "$PLUGIN_STORE_ADDR" "$AUTOSTART" "$RC_LOCAL" \
      "${UFI_DEVICE_BACKEND:-u60-local}" "${UFI_DEVICE_BASE_URL-}" "${UFI_WORKERS_ENABLED:-false}" "${UFI_PRIVILEGED_COMPAT:-false}" "${UFI_AT_ENABLED:-false}" > "$target/.ufi-runtime"
  )
}

require_safe_root() {
  for tree in "$ROOT" "$NEXT" "$BACKUP" "$UNINSTALLED" "$RESTORE_RESERVE"; do
    [ ! -L "$tree" ] || { echo "refusing symlink installation tree: $tree" >&2; return 1; }
    if [ "$tree" != "$ROOT" ] && [ -d "$tree" ] && [ -n "$(ls -A "$tree")" ]; then
      [ -f "$tree/$INVENTORY_NAME" ] && grep -Fqx '# ufi-tools managed installation' "$tree/$INVENTORY_NAME" || {
        echo "refusing to change an unowned recovery directory: $tree" >&2; return 1;
      }
    fi
  done
  case "$ROOT" in
    /data/ufi-tools|/data/ufi-tools/*|/data/ufi-tools.*)
      echo "the original UFI-TOOLS tree is outside this installer's scope" >&2; return 1 ;;
    /|/data|/data/|/data/local/tmp|/data/local/tmp/|/tmp|/tmp/|/sdcard|/sdcard/|/etc|/etc/|/system|/system/|/usr|/usr/)
      echo "refusing unsafe install root: $ROOT" >&2
      return 1
      ;;
    *..*|*//*|*/./*|*/.|*/|*[!a-zA-Z0-9_./-]*)
      echo "refusing install root containing '..': $ROOT" >&2
      return 1
      ;;
    /*) return 0 ;;
    *)
      echo "install root must be absolute: $ROOT" >&2
      return 1
      ;;
  esac
}

verify_bundle() {
  [ -f "$STAGE/SHA256SUMS" ] || { echo "missing SHA256SUMS" >&2; return 1; }
  [ -z "$(find "$STAGE" -type l -print)" ] || { echo "bundle symlinks are not supported" >&2; return 1; }
  awk '
    NF!=2 || length($1)!=64 || $1 ~ /[^0-9a-f]/ || $2 ~ /[^a-zA-Z0-9_.\/-]/ ||
    $2 ~ /^\// || $2 ~ /(^|\/)\.\.?($|\/)/ || seen[$2]++ { bad=1 }
    { files[$2]=1 }
    END { exit bad || !files["ufi-tools-u60pro"] || !files["service.sh"] }' "$STAGE/SHA256SUMS" || return 1
  listed=$(wc -l < "$STAGE/SHA256SUMS" | tr -d ' ')
  actual=$(find "$STAGE" -type f ! -path "$STAGE/SHA256SUMS" | wc -l | tr -d ' ')
  [ "$listed" = "$actual" ] || { echo "bundle inventory mismatch" >&2; return 1; }
  while read -r expected name; do
    [ -f "$STAGE/$name" ] || return 1
    if command -v sha256sum >/dev/null 2>&1; then
      actual_hash=$(sha256sum "$STAGE/$name" | awk '{print $1}')
    else
      actual_hash=$(shasum -a 256 "$STAGE/$name" | awk '{print $1}')
    fi
    [ "$actual_hash" = "$expected" ] || { echo "checksum mismatch: $name" >&2; return 1; }
  done < "$STAGE/SHA256SUMS"
  if [ -e "$STAGE/manifest.json" ]; then
    chmod 755 "$STAGE/verify-release" || return 1
    "$STAGE/verify-release" -manifest "$STAGE/manifest.json" -artifact "$STAGE/ufi-tools-u60pro" -public-key "$STAGE/public-key.hex" || return 1
  fi
}

check_install_space() {
  bundle_kb=$(du -sk "$STAGE" | awk '{print $1}')
  active_kb=0
  [ ! -d "$ROOT" ] || active_kb=$(du -sk "$ROOT" | awk '{print $1}')
  available=$(df -Pk "$(dirname "$ROOT")" | awk 'NR==2 {print $4}')
  case "$available:$bundle_kb:$active_kb" in *[!0-9:]*) echo "cannot determine available space" >&2; return 1 ;; esac
  [ -n "$available" ] || return 1
  needed=$((bundle_kb + active_kb + 16384))
  [ "$available" -ge "$needed" ] || { echo "insufficient space: need $needed KiB, have $available" >&2; return 1; }
}

remove_staging() {
  case "$STAGING" in
    ''|/|/data|/data/|/data/local/tmp|/data/local/tmp/|/tmp|/tmp/) return 0 ;;
  esac
  case "$STAGING/" in
    "$ROOT/"*|"$BACKUP/"*|"$NEXT/"*|"$UNINSTALLED/"*|"$RESTORE_RESERVE/"*)
      echo "refusing staging cleanup inside installation" >&2; return 1 ;;
  esac
  case "$ROOT/" in
    "$STAGING/"*) echo "refusing staging cleanup above installation" >&2; return 1 ;;
  esac
  if [ -f "$STAGING.owner" ] && [ "$(cat "$STAGING.owner")" = "$ROOT" ]; then
    rm -rf "$STAGING"
    rm -f "$STAGING.owner" "$STAGING.controller"
  fi
}

is_owned_root() {
  [ -f "$ROOT/$INVENTORY_NAME" ] && grep -Fqx '# ufi-tools managed installation' "$ROOT/$INVENTORY_NAME"
}

require_owned_root() {
  [ -d "$ROOT" ] || return 0
  [ -n "$(ls -A "$ROOT" 2>/dev/null)" ] || return 0
  if is_owned_root; then
    for managed in data .ufi-system .ufi-runtime service.log ufi-tools.pid .ufi-health-instance data/ufi-tools.db data/device-token; do
      [ ! -L "$ROOT/$managed" ] || { echo "refusing linked mutable state: $managed" >&2; return 1; }
    done
    return 0
  fi
  echo "refusing to modify unowned directory: $ROOT" >&2
  return 1
}

write_inventory() {
  target=$1
  {
    printf '%s\n' '# ufi-tools managed installation'
    for entry in ufi-tools-u60pro verify-release public-key.hex service.sh manifest.json SHA256SUMS ttyd speedtest sendat ufi-plugin-store plugin-store data config.json sdcard sdcard.img .ufi-runtime .ufi-system lan-plan-id; do
      if [ -e "$target/$entry" ]; then
        printf '%s\n' "$entry"
      fi
    done
  } > "$target/$INVENTORY_NAME"
}

recover_interrupted() {
  require_safe_root || return 1
  check_tree_mounts || return 1
  if [ -d "$ROOT" ] && [ -f "$PENDING" ] && [ -d "$BACKUP" ]; then
    echo "rolling back unverified installation from $ROOT" >&2
    stop || return 1
    release_plugin_storage || return 1
    rm -rf "$ROOT"
    mv "$BACKUP" "$ROOT" || return 1
    RECOVERED=1
  elif [ ! -d "$ROOT" ] && [ -d "$BACKUP" ]; then
    echo "recovering interrupted installation from $BACKUP" >&2
    mv "$BACKUP" "$ROOT" || return 1
    RECOVERED=1
  fi
  if [ -d "$NEXT" ]; then
    rm -rf "$NEXT"
  fi
  return 0
}

install_abort() {
  [ "$INSTALL_ACTIVE" = 1 ] || return 0
  INSTALL_ACTIVE=0
  trap - 0 1 2 15 2>/dev/null || true
  if [ "$INSTALL_STOPPED" = 1 ] || [ "$INSTALL_ROTATED" = 1 ]; then
    stop || { echo "rollback deferred: service did not stop" >&2; return 1; }
  fi
  if [ "$AUTOSTART_TOUCHED" = 1 ]; then restore_rc "$RC_TX" || return 1; fi
  if [ "$INSTALL_ROTATED" = 1 ]; then
    if ! release_plugin_storage; then
      echo "rollback deferred: mount must be released before recovery" >&2
      return 1
    fi
    rm -rf "$ROOT"
    if [ -d "$BACKUP" ]; then
      mv "$BACKUP" "$ROOT" || true
    fi
  fi
  rm -rf "$NEXT"
  rm -rf "$RC_TX"
  if [ "$INSTALL_STOPPED" = 1 ] && [ -x "$ROOT/ufi-tools-u60pro" ]; then
    start_restored || true
  fi
  return 0
}

start_restored() {
  # An older/closed service has its own environment and PID-file conventions.
  if [ -f "$ROOT/service.sh" ]; then
    sh "$ROOT/service.sh" start || return 1
  else
    # Retain support for old snapshots containing only the executable and data.
    RECOVERED=0
    start || return 1
  fi
  sleep "$HEALTH_SETTLE_SECONDS"
  if [ -f "$INSTANCE_FILE" ] && is_running "$PIDFILE"; then
    health
  else
    is_running "$LEGACY_PIDFILE" || return 1
    # Legacy releases do not expose an instance identity. Confirm liveness and
    # HTTP readiness, without claiming the new install's stronger identity proof.
    wget -q -T 3 -O - "$HEALTH_URL" >/dev/null 2>&1
  fi
}

install_begin() {
  INSTALL_ACTIVE=1
  INSTALL_STOPPED=0
  INSTALL_ROTATED=0
  AUTOSTART_TOUCHED=0
  install_status=0
  trap 'install_status=$?; install_abort; exit "$install_status"' 0
  trap 'install_abort; exit 1' 1 2 15
}

install_finish() {
  rm -rf "$RC_TX"
  INSTALL_ACTIVE=0
  trap - 0 1 2 15
}

migrate_state() {
  if [ -d "$ROOT/.ufi-system" ]; then cp -Rp "$ROOT/.ufi-system" "$NEXT/.ufi-system"; fi
  # These are the persistent files of the installation; keep this list in
  # sync with anything that must survive an upgrade.
  mkdir -p "$NEXT/data"
  if [ -d "$ROOT/data" ]; then
    cp -Rp "$ROOT/data/." "$NEXT/data/"
  fi
  for state_file in config.json sdcard.img; do
    if [ -f "$ROOT/$state_file" ]; then
      cp -p "$ROOT/$state_file" "$NEXT/$state_file"
    fi
  done
  if [ -d "$ROOT/sdcard" ]; then
    mkdir -p "$NEXT/sdcard"
    cp -Rp "$ROOT/sdcard/." "$NEXT/sdcard/"
  fi
  if [ ! -d "$NEXT/plugin-store" ] && [ -d "$ROOT/plugin-store" ]; then
    cp -Rp "$ROOT/plugin-store" "$NEXT/plugin-store"
  fi
}

install_bundle() {
  STAGE=${1:?stage directory required}
  require_safe_root || return 1
  recover_interrupted || return 1
  require_owned_root || return 1
  verify_bundle || return 1
  check_install_space || return 1
  [ ! -e "$RC_TX" ] || { echo "previous boot transaction needs inspection: $RC_TX" >&2; return 1; }
  install_begin
  [ "$AUTOSTART" != 1 ] || capture_rc "$RC_TX"

  autostart_was_present=0
  if [ "$AUTOSTART" = 1 ]; then
    [ "$(rc_line_count)" -eq 0 ] || autostart_was_present=1
  fi

  rm -rf "$NEXT"
  mkdir -p "$NEXT"
  chmod 700 "$NEXT" || return 1
  printf '%s\n' '# ufi-tools managed installation' > "$NEXT/$INVENTORY_NAME"
  cp "$STAGE/ufi-tools-u60pro" "$NEXT/ufi-tools-u60pro"
  cp "$STAGE/service.sh" "$NEXT/service.sh"
  cp "$STAGE/SHA256SUMS" "$NEXT/SHA256SUMS"
  for metadata in verify-release public-key.hex manifest.json LICENSE NOTICE build-info.txt lan-plan-id; do
    [ ! -f "$STAGE/$metadata" ] || cp "$STAGE/$metadata" "$NEXT/$metadata"
  done
  for optional in ttyd speedtest sendat ufi-plugin-store; do
    if [ -f "$STAGE/$optional" ]; then
      cp "$STAGE/$optional" "$NEXT/$optional"
    elif [ -f "$ROOT/$optional" ]; then
      cp "$ROOT/$optional" "$NEXT/$optional"
    fi
  done
  if [ -d "$STAGE/plugin-store" ]; then
    cp -R "$STAGE/plugin-store" "$NEXT/plugin-store"
  fi
  chmod 0755 "$NEXT/ufi-tools-u60pro" "$NEXT/service.sh"
  [ ! -f "$NEXT/verify-release" ] || chmod 0755 "$NEXT/verify-release"
  [ ! -f "$NEXT/ttyd" ] || chmod 0755 "$NEXT/ttyd"
  [ ! -f "$NEXT/speedtest" ] || chmod 0755 "$NEXT/speedtest"
  [ ! -f "$NEXT/sendat" ] || chmod 0755 "$NEXT/sendat"
  [ ! -f "$NEXT/ufi-plugin-store" ] || chmod 0755 "$NEXT/ufi-plugin-store"

  INSTALL_STOPPED=1
  if ! stop; then
    echo "failed to stop the running service; refusing to migrate data" >&2
    return 1
  fi
  release_plugin_storage || return 1
  migrate_state
  chmod 700 "$NEXT/data"
  if [ -f "$STAGE/lan-login-token" ] && [ ! -e "$NEXT/data/ufi-tools.db" ]; then
    cp "$STAGE/lan-login-token" "$NEXT/data/initial-device-token"
    chmod 600 "$NEXT/data/initial-device-token"
  fi
  write_runtime "$NEXT"
  write_inventory "$NEXT"

  rm -rf "$BACKUP"
  [ ! -d "$ROOT" ] || mv "$ROOT" "$BACKUP"
  INSTALL_ROTATED=1
  : > "$NEXT/.ufi-tools-pending"
  mv "$NEXT" "$ROOT"

  if start && health; then
    if [ "$AUTOSTART" != 1 ]; then
      if is_running "$PIDFILE"; then
        rm -f "$PENDING"
        install_finish
        return 0
      fi
      echo "candidate exited during the installation" >&2
    else
      if enable_autostart && is_running "$PIDFILE"; then
        rm -f "$PENDING"
        install_finish
        return 0
      fi
      echo "failed to install the autostart entry in $RC_LOCAL" >&2
    fi
  fi
  return 1
}

uninstall() {
  mode=keep-backup
  case "${1:-}" in
    ''|--keep-backup) mode=keep-backup ;;
    --keep-data) mode=keep-data ;;
    --purge) mode=purge ;;
    --restore) mode=restore ;;
    *)
      echo "usage: $0 uninstall [--keep-data|--keep-backup|--purge|--restore]" >&2
      return 2
      ;;
  esac
  [ "$#" -le 1 ] || { echo "too many uninstall arguments" >&2; return 2; }

  require_safe_root || return 1
  require_owned_root || return 1

  if [ "$mode" = restore ]; then
    restore_from=$BACKUP
    [ -d "$restore_from" ] || restore_from=$UNINSTALLED
    if [ ! -d "$restore_from" ]; then
      echo "no previous installation to restore: $BACKUP" >&2
      return 1
    fi
    [ ! -e "$RESTORE_RESERVE" ] || { echo "previous restore reserve exists: $RESTORE_RESERVE" >&2; return 1; }
    if ! stop; then
      echo "refusing to restore while service processes are still running" >&2
      return 1
    fi
    release_plugin_storage || return 1
    [ ! -d "$ROOT" ] || mv "$ROOT" "$RESTORE_RESERVE" || return 1
    if ! mv "$restore_from" "$ROOT"; then
      [ ! -d "$RESTORE_RESERVE" ] || mv "$RESTORE_RESERVE" "$ROOT"
      [ ! -d "$ROOT" ] || start_restored || true
      echo "failed to restore $BACKUP to $ROOT" >&2
      return 1
    fi
    rm -rf "$NEXT"
    remove_staging
    if ! start_restored; then
      echo "restored service failed readiness; preserving it and recovering the active tree" >&2
      stop || return 1
      release_plugin_storage || return 1
      mv "$ROOT" "$restore_from" || return 1
      if [ -d "$RESTORE_RESERVE" ]; then
        mv "$RESTORE_RESERVE" "$ROOT" || return 1
        start_restored || true
      fi
      return 1
    fi
    [ "$AUTOSTART" != 1 ] || enable_autostart || return 1
    return 0
  fi

  if [ "$mode" = keep-backup ] && [ -d "$ROOT" ] && [ -e "$UNINSTALLED" ]; then
    echo "saved uninstall already exists: $UNINSTALLED; refusing to overwrite it" >&2
    return 1
  fi

  if [ ! -d "$ROOT" ] && [ -d "$BACKUP" ] && [ ! -d "$UNINSTALLED" ] && [ "$mode" != purge ]; then
    echo "refusing: $ROOT is missing but $BACKUP exists (interrupted install); use uninstall --restore" >&2
    return 1
  fi

  if ! stop; then
    echo "refusing to uninstall while service processes are still running" >&2
    return 1
  fi
  release_plugin_storage || return 1

  case "$mode" in
    keep-data)
      if [ "$AUTOSTART" = 1 ]; then
        disable_autostart || {
          echo "failed to remove the autostart entry from $RC_LOCAL" >&2
          return 1
        }
      fi
      rm -rf "$NEXT"
      remove_staging
      echo "service stopped; kept $ROOT"
      ;;
    keep-backup)
      if [ "$AUTOSTART" = 1 ]; then
        disable_autostart || {
          echo "failed to remove the autostart entry from $RC_LOCAL" >&2
          return 1
        }
      fi
      [ ! -d "$ROOT" ] || mv "$ROOT" "$UNINSTALLED"
      rm -rf "$NEXT"
      remove_staging
      ;;
    purge)
      if [ "$AUTOSTART" = 1 ]; then
        disable_autostart || {
          echo "failed to remove the autostart entry from $RC_LOCAL" >&2
          return 1
        }
      fi
      rm -rf "$ROOT" "$BACKUP" "$NEXT" "$UNINSTALLED" "$RESTORE_RESERVE"
      remove_staging
      ;;
  esac
}

lan_bootstrap() {
  STAGE=${1:?LAN staging directory required}
  case "$STAGE" in "$ROOT.lan-stage-"*) ;; *) echo "unexpected LAN staging path" >&2; return 1 ;; esac
  suffix=${STAGE#"$ROOT.lan-stage-"}
  case "$suffix" in ''|*[!a-z0-9-]*) return 1 ;; esac
  verify_bundle || return 1
  deployment=$(cat "$STAGE/lan-plan-id")
  [ "${#deployment}" = 32 ] || return 1
  case "$deployment" in *[!0-9a-f]*) return 1 ;; esac
  action=$(cat "$STAGE/lan-action")
  # The temporary hook is removed before the normal installer snapshots the
  # boot file, so later uninstall restores the genuine pre-LAN baseline.
  cmp -s "$RC_LOCAL" "$STAGE/lan-rc-after" || { echo "LAN boot file differs from prepared baseline" >&2; return 1; }
  write_rc_content "$STAGE/lan-rc-before" || return 1
  case "$action" in
    install)
      for tree in "$ROOT" "$NEXT" "$BACKUP" "$UNINSTALLED" "$RESTORE_RESERVE"; do
        [ ! -e "$tree" ] || { echo "LAN supports first install only; use ADB for an existing candidate" >&2; return 1; }
      done
      # Call as a command, not an if-condition: install relies on errexit traps.
      install_bundle "$STAGE"
      ;;
    remove)
      if [ -d "$ROOT" ]; then
        [ "$(cat "$ROOT/lan-plan-id" 2>/dev/null)" = "$deployment" ] || { echo "LAN deployment mismatch" >&2; return 1; }
        uninstall --keep-backup || return 1
      fi
      ;;
    restore)
      # The prepared boot preimage already describes the user's latest choice.
      # Restoring the candidate must not re-enable a line the user removed.
      AUTOSTART=0
      if [ -d "$ROOT" ]; then
        [ "$(cat "$ROOT/lan-plan-id" 2>/dev/null)" = "$deployment" ] || return 1
        start_restored || return 1
      else
        [ ! -e "$BACKUP" ] || return 1
        [ "$(cat "$UNINSTALLED/lan-plan-id" 2>/dev/null)" = "$deployment" ] || return 1
        uninstall --restore || return 1
      fi
      ;;
    *) return 1 ;;
  esac
  rm -rf "$STAGE"
}

with_lifecycle_lock() (
  require_safe_root || exit 1
  lock=${ROOT}.lifecycle-lock
  if ! mkdir "$lock" 2>/dev/null; then
    echo "lifecycle transaction already active or interrupted: $lock" >&2
    exit 1
  fi
  trap 'rm -f "$lock/owner"; rmdir "$lock" 2>/dev/null || true' 0
  trap 'exit 1' 1 2 15
  UFI_LIFECYCLE_OWNER=$$
  export UFI_LIFECYCLE_OWNER
  printf '%s\n' "$UFI_LIFECYCLE_OWNER" > "$lock/owner"
  # Keep this lock's cleanup trap outside the install transaction's own traps.
  ( "$@" )
)

if [ -d "${ROOT}.lifecycle-lock" ]; then
  owner=$(cat "${ROOT}.lifecycle-lock/owner" 2>/dev/null || true)
  if [ -z "$owner" ] || [ "${UFI_LIFECYCLE_OWNER:-}" != "$owner" ]; then
    echo "lifecycle transaction already active or interrupted: ${ROOT}.lifecycle-lock" >&2
    exit 1
  fi
fi

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  health) health ;;
  install) shift; with_lifecycle_lock install_bundle "$@" ;;
  uninstall) shift; with_lifecycle_lock uninstall "$@" ;;
  lan-bootstrap) shift; with_lifecycle_lock lan_bootstrap "$@" ;;
  enable) with_lifecycle_lock enable_autostart ;;
  disable) with_lifecycle_lock disable_autostart ;;
  *) echo "usage: $0 {start|stop|restart|health|install STAGE|uninstall [MODE]|enable|disable}" >&2; exit 2 ;;
esac
