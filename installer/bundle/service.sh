#!/bin/sh
set -eu

ROOT=${UFI_INSTALL_ROOT:-/data/ufi-tools}
NEXT=${ROOT}.next
BACKUP=${ROOT}.previous
PIDFILE=$ROOT/ufi-tools.pid
HEALTH_URL=${UFI_HEALTH_URL:-http://127.0.0.1:2333/healthz}
HEALTH_ATTEMPTS=${UFI_HEALTH_ATTEMPTS:-20}

start() {
  [ -x "$ROOT/ufi-tools-u60pro" ] || exit 1
  if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then return 0; fi
  UFI_DATA_DIR="$ROOT/data" UFI_HTTP_ADDR=:2333 "$ROOT/ufi-tools-u60pro" >>"$ROOT/service.log" 2>&1 &
  echo $! > "$PIDFILE"
}

stop() {
  if [ -f "$PIDFILE" ]; then
    kill "$(cat "$PIDFILE")" 2>/dev/null || true
    rm -f "$PIDFILE"
  fi
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
  chmod 0755 "$NEXT/ufi-tools-u60pro" "$NEXT/verify-release" "$NEXT/service.sh"
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
