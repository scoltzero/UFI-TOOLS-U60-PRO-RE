#!/bin/sh
set -eu

ROOT=${UFI_INSTALL_ROOT:-/data/ufi-tools}
NEXT=${ROOT}.next
BACKUP=${ROOT}.previous
PIDFILE=$ROOT/ufi-tools.pid
LEGACY_PIDFILE=$ROOT/ufi-tools-u60pro.pid
STORE_PIDFILE=$ROOT/ufi-plugin-store.pid
HEALTH_URL=${UFI_HEALTH_URL:-http://127.0.0.1:2333/healthz}
HEALTH_ATTEMPTS=${UFI_HEALTH_ATTEMPTS:-20}
PLUGIN_BOOT_MARK=${UFI_PLUGIN_BOOT_MARK:-/tmp/.ufi-plugin-boot-done}
PLUGIN_MOUNT=${UFI_PLUGIN_MOUNT:-/sdcard}
PLUGIN_STORAGE=$ROOT/sdcard
PLUGIN_IMAGE=$ROOT/sdcard.img

is_running() {
  [ -f "$1" ] && kill -0 "$(cat "$1")" 2>/dev/null
}

is_mounted() {
  awk -v target="$1" '$2 == target { found=1 } END { exit !found }' /proc/mounts 2>/dev/null
}

plugin_storage_enabled() {
  [ "${UFI_PLUGIN_STORAGE_ENABLED:-$([ "$ROOT" = /data/ufi-tools ] && echo true || echo false)}" = true ]
}

prepare_plugin_storage() {
  plugin_storage_enabled || return 0
  is_mounted "$PLUGIN_MOUNT" && return 0
  mkdir -p "$PLUGIN_STORAGE" "$PLUGIN_MOUNT"
  if [ -n "$(ls -A "$PLUGIN_MOUNT" 2>/dev/null)" ]; then
    cp -a "$PLUGIN_MOUNT/." "$PLUGIN_STORAGE/" 2>/dev/null || true
  fi
  if mount --bind "$PLUGIN_STORAGE" "$PLUGIN_MOUNT" 2>/dev/null; then return 0; fi
  if command -v mkfs.ext4 >/dev/null 2>&1 && command -v dd >/dev/null 2>&1; then
    if [ ! -f "$PLUGIN_IMAGE" ]; then
      dd if=/dev/zero of="$PLUGIN_IMAGE" bs=1M count=64 >/dev/null 2>&1 || return 0
      mkfs.ext4 -F -q "$PLUGIN_IMAGE" >/dev/null 2>&1 || return 0
    fi
    mount -t ext4 -o loop "$PLUGIN_IMAGE" "$PLUGIN_MOUNT" 2>/dev/null || true
  fi
}

run_plugin_boot() {
  plugin_storage_enabled || return 0
  [ -e "$PLUGIN_BOOT_MARK" ] && return 0
  if [ -s "$PLUGIN_MOUNT/ufi_tools_boot.sh" ]; then
    sh "$PLUGIN_MOUNT/ufi_tools_boot.sh" >>"$ROOT/plugin-boot.log" 2>&1 || true
  fi
  : > "$PLUGIN_BOOT_MARK"
}

start_store() {
  [ -x "$ROOT/ufi-plugin-store" ] || return 0
  [ -f "$ROOT/plugin-store/catalog.json" ] || return 0
  is_running "$STORE_PIDFILE" && return 0
  "$ROOT/ufi-plugin-store" -root "$ROOT/plugin-store" -http-addr 127.0.0.1:2340 >>"$ROOT/plugin-store.log" 2>&1 &
  echo $! > "$STORE_PIDFILE"
}

start() {
  [ -x "$ROOT/ufi-tools-u60pro" ] || exit 1
  is_running "$PIDFILE" && return 0
  prepare_plugin_storage
  run_plugin_boot
  start_store
  store_url=${UFI_PLUGIN_SERVER_URL:-}
  if [ -z "$store_url" ] && is_running "$STORE_PIDFILE"; then store_url=http://127.0.0.1:2340; fi
  UFI_DATA_DIR="$ROOT/data" \
  UFI_HTTP_ADDR=:2333 \
  UFI_HOSTS_PATH=/etc/hosts \
  UFI_DEVICE_BACKEND="${UFI_DEVICE_BACKEND:-u60-ubus}" \
  UFI_DEVICE_BASE_URL="${UFI_DEVICE_BASE_URL:-http://127.0.0.1}" \
  UFI_WORKERS_ENABLED="${UFI_WORKERS_ENABLED:-true}" \
  UFI_PRIVILEGED_COMPAT="${UFI_PRIVILEGED_COMPAT:-true}" \
  UFI_AT_ENABLED="${UFI_AT_ENABLED:-true}" \
  UFI_PLUGIN_SERVER_URL="$store_url" \
  "$ROOT/ufi-tools-u60pro" >>"$ROOT/service.log" 2>&1 &
  echo $! > "$PIDFILE"
}

stop() {
  for pidfile in "$PIDFILE" "$LEGACY_PIDFILE" "$STORE_PIDFILE"; do
    if [ -f "$pidfile" ]; then
      kill "$(cat "$pidfile")" 2>/dev/null || true
      rm -f "$pidfile"
    fi
  done
  pkill -f "$ROOT/ufi-tools-u60pro" 2>/dev/null || true
}

health() {
  i=0
  while [ "$i" -lt "$HEALTH_ATTEMPTS" ]; do
    if wget -q -O - "$HEALTH_URL" >/dev/null 2>&1; then return 0; fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

install_bundle() {
  STAGE=${1:?stage directory required}
  "$STAGE/verify-release" -manifest "$STAGE/manifest.json" -artifact "$STAGE/ufi-tools-u60pro" -public-key "$STAGE/public-key.hex"
  rm -rf "$NEXT"
  mkdir -p "$NEXT/data"
  if [ -d "$ROOT/data" ]; then cp -R "$ROOT/data/." "$NEXT/data/"; fi
  cp "$STAGE/ufi-tools-u60pro" "$NEXT/ufi-tools-u60pro"
  cp "$STAGE/verify-release" "$NEXT/verify-release"
  cp "$STAGE/public-key.hex" "$NEXT/public-key.hex"
  cp "$STAGE/service.sh" "$NEXT/service.sh"
  cp "$STAGE/manifest.json" "$NEXT/manifest.json"
  for optional in ttyd speedtest sendat ufi-plugin-store; do
    if [ -f "$STAGE/$optional" ]; then
      cp "$STAGE/$optional" "$NEXT/$optional"
    elif [ -f "$ROOT/$optional" ]; then
      cp "$ROOT/$optional" "$NEXT/$optional"
    fi
  done
  if [ -d "$STAGE/plugin-store" ]; then
    cp -R "$STAGE/plugin-store" "$NEXT/plugin-store"
  elif [ -d "$ROOT/plugin-store" ]; then
    cp -R "$ROOT/plugin-store" "$NEXT/plugin-store"
  fi
  chmod 0755 "$NEXT/ufi-tools-u60pro" "$NEXT/verify-release" "$NEXT/service.sh"
  [ ! -f "$NEXT/ttyd" ] || chmod 0755 "$NEXT/ttyd"
  [ ! -f "$NEXT/speedtest" ] || chmod 0755 "$NEXT/speedtest"
  [ ! -f "$NEXT/sendat" ] || chmod 0755 "$NEXT/sendat"
  [ ! -f "$NEXT/ufi-plugin-store" ] || chmod 0755 "$NEXT/ufi-plugin-store"
  stop
  rm -rf "$BACKUP"
  [ ! -d "$ROOT" ] || mv "$ROOT" "$BACKUP"
  mv "$NEXT" "$ROOT"
  if start && health; then
    exit 0
  fi
  stop
  rm -rf "$ROOT"
  if [ -d "$BACKUP" ]; then
    mv "$BACKUP" "$ROOT"
    start || true
  fi
  exit 1
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  health) health ;;
  install) shift; install_bundle "$@" ;;
  *) echo "usage: $0 {start|stop|restart|health|install STAGE}" >&2; exit 2 ;;
esac
