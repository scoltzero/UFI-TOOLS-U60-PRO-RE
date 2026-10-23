#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
SERVICE=$PROJECT_ROOT/installer/bundle/service.sh
TMP=$(mktemp -d)
ROOT=$TMP/root
BUNDLE=$TMP/bundle
TOOLS=$TMP/tools
RC_LOCAL=$TMP/etc/rc.local
STAGING=$TMP/staging
BOOT_MARK=$TMP/boot-marker
SIDE_ROOT=$TMP/side-root
SIDE_BUNDLE=$TMP/side-bundle
SIDE_STAGING=$TMP/side-staging
ORIGINAL_ROOT=$TMP/original
FAIL_ROOT=$TMP/fail-root
FAIL_BUNDLE=$TMP/fail-bundle
FAIL_STAGING=$TMP/fail-staging
FAIL_TOOLS=$TMP/fail-tools
RC_LINE="sh $ROOT/service.sh start"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  PATH="$TOOLS:$PATH" \
    UFI_INSTALL_ROOT="$ROOT" \
    UFI_RC_LOCAL="$RC_LOCAL" \
    UFI_HOSTS_PATH='' \
    UFI_PLUGIN_STORAGE_ENABLED=false \
    UFI_PLUGIN_BOOT_MARK="$BOOT_MARK" \
    "$BUNDLE/service.sh" stop >/dev/null 2>&1 || true
  PATH="$TOOLS:$PATH" \
    UFI_INSTALL_ROOT="$SIDE_ROOT" \
    UFI_RC_LOCAL="$SIDE_ROOT.rc.local" \
    UFI_HOSTS_PATH='' \
    UFI_PLUGIN_STORAGE_ENABLED=false \
    "$SERVICE" stop >/dev/null 2>&1 || true
  [ -z "${ORIGINAL_PID:-}" ] || kill "$ORIGINAL_PID" 2>/dev/null || true
  [ -z "${FAIL_OLD_PID:-}" ] || kill "$FAIL_OLD_PID" 2>/dev/null || true
  if [ -f "$FAIL_ROOT/ufi-tools.pid" ]; then
    kill "$(cat "$FAIL_ROOT/ufi-tools.pid")" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

run_service() {
  script=$1
  shift
  env \
    PATH="$TOOLS:$PATH" \
    UFI_INSTALL_ROOT="$ROOT" \
    UFI_RC_LOCAL="$RC_LOCAL" \
    UFI_HOSTS_PATH='' \
    UFI_PLUGIN_STORAGE_ENABLED=false \
    UFI_PLUGIN_BOOT_MARK="$BOOT_MARK" \
    UFI_STAGING_DIR="$STAGING" \
    UFI_HEALTH_ATTEMPTS="${UFI_HEALTH_ATTEMPTS:-1}" \
    UFI_STOP_ATTEMPTS="${UFI_STOP_ATTEMPTS:-5}" \
    UFI_HEALTH_SETTLE_SECONDS="${UFI_HEALTH_SETTLE_SECONDS:-1}" \
    "$script" "$@"
}

checksum_bundle() {
  (cd "$1"; find . -type f ! -name SHA256SUMS | sed 's|^./||' | LC_ALL=C sort | while read -r name; do
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$name"; else shasum -a 256 "$name"; fi
  done > SHA256SUMS)
}

run_install() {
  checksum_bundle "$BUNDLE"
  run_service "$BUNDLE/service.sh" install "$BUNDLE"
}

run_side() {
  [ "${2:-}" != install ] || checksum_bundle "$SIDE_BUNDLE"
  script=$1
  shift
  env \
    PATH="$TOOLS:$PATH" \
    UFI_INSTALL_ROOT="$SIDE_ROOT" \
    UFI_RC_LOCAL="$SIDE_ROOT.rc.local" \
    UFI_HTTP_ADDR=:2433 \
    UFI_HEALTH_URL=http://127.0.0.1:2433/healthz \
    UFI_HOSTS_PATH='' \
    UFI_TTYD_BINARY="$SIDE_ROOT/ttyd" \
    UFI_TTYD_PORT=1147 \
    UFI_SPEEDTEST_BINARY="$SIDE_ROOT/speedtest" \
    UFI_AT_BINARY="$SIDE_ROOT/sendat" \
    UFI_PLUGIN_STORE_ADDR=127.0.0.1:2341 \
    UFI_AUTOSTART=0 \
    UFI_PLUGIN_STORAGE_ENABLED=false \
    UFI_STAGING_DIR="$SIDE_STAGING" \
    UFI_HEALTH_ATTEMPTS=1 \
    UFI_STOP_ATTEMPTS=5 \
    UFI_HEALTH_SETTLE_SECONDS=1 \
    "$script" "$@"
}

run_fail() {
  [ "${2:-}" != install ] || checksum_bundle "$FAIL_BUNDLE"
  script=$1
  shift
  env \
    PATH="$FAIL_TOOLS:$TOOLS:$PATH" \
    UFI_INSTALL_ROOT="$FAIL_ROOT" \
    UFI_RC_LOCAL="$FAIL_ROOT.rc.local" \
    UFI_HOSTS_PATH='' \
    UFI_PLUGIN_STORAGE_ENABLED=false \
    UFI_AUTOSTART=0 \
    UFI_PLUGIN_BOOT_MARK="$BOOT_MARK" \
    UFI_STAGING_DIR="$FAIL_STAGING" \
    UFI_HEALTH_ATTEMPTS=1 \
    UFI_STOP_ATTEMPTS=5 \
    UFI_HEALTH_SETTLE_SECONDS=1 \
    "$script" "$@"
}

stop_fail_root() {
  [ -z "${FAIL_OLD_PID:-}" ] || kill "$FAIL_OLD_PID" 2>/dev/null || true
  FAIL_OLD_PID=
  if [ -f "$FAIL_ROOT/ufi-tools.pid" ]; then
    kill "$(cat "$FAIL_ROOT/ufi-tools.pid")" 2>/dev/null || true
  fi
}

reset_fail_root() {
  stop_fail_root
  rm -rf "$FAIL_ROOT" "$FAIL_ROOT.previous" "$FAIL_ROOT.next" "$FAIL_ROOT.rc.local" "$FAIL_STAGING"
  mkdir -p "$FAIL_ROOT/data"
  printf '%s\n' '# ufi-tools managed installation' > "$FAIL_ROOT/.ufi-tools-manifest"
  printf '%s\n' fail-old > "$FAIL_ROOT/data/persist-marker"
  printf '%s\n' fail-old > "$FAIL_ROOT/config.json"
  printf '%s\n' fail-old > "$FAIL_ROOT/version-marker"
  cat > "$FAIL_ROOT/ufi-tools-u60pro" <<'EOF'
#!/bin/sh
trap 'exit 0' TERM
: > "$UFI_DATA_DIR/ready-marker"
while :; do sleep 1; done
EOF
  chmod 0755 "$FAIL_ROOT/ufi-tools-u60pro"
  UFI_DATA_DIR="$FAIL_ROOT/data" "$FAIL_ROOT/ufi-tools-u60pro" &
  FAIL_OLD_PID=$!
  echo "$FAIL_OLD_PID" > "$FAIL_ROOT/ufi-tools-u60pro.pid"
  i=0
  while [ ! -f "$FAIL_ROOT/data/ready-marker" ] && [ "$i" -lt 50 ]; do
    i=$((i + 1))
    sleep 0.1
  done
  assert_present "$FAIL_ROOT/data/ready-marker" "failure fixture did not become ready"
}

assert_fail_root_restored() {
  [ "$(cat "$FAIL_ROOT/data/persist-marker" 2>/dev/null || true)" = fail-old ] || fail "$1: data marker was not restored"
  assert_present "$FAIL_ROOT/ufi-tools.pid" "$1: previous service was not restarted"
  fail_pid=$(cat "$FAIL_ROOT/ufi-tools.pid")
  kill -0 "$fail_pid" 2>/dev/null || fail "$1: previous service is not running"
  [ ! -e "$FAIL_ROOT.next" ] || fail "$1: staging tree was left behind"
  [ ! -e "$FAIL_ROOT.rc.local" ] || fail "$1: autostart was created"
}

rc_count() {
  if [ -f "$RC_LOCAL" ]; then
    awk -v line="$RC_LINE" '$0 == line { n++ } END { print n + 0 }' "$RC_LOCAL"
  else
    echo 0
  fi
}

assert_eq() {
  [ "$1" = "$2" ] || fail "$3 (got '$1', want '$2')"
}

assert_present() {
  [ -e "$1" ] || fail "$2"
}

mkdir -p "$ROOT/data" "$ROOT/sdcard" "$BUNDLE" "$TOOLS" "$(dirname "$RC_LOCAL")" "$STAGING"

printf '%s\n' '# ufi-tools managed installation' > "$ROOT/.ufi-tools-manifest"
printf '%s\n' "$ROOT" > "$STAGING.owner"
printf '#!/bin/sh\n# original system startup\nexit 0\n' > "$RC_LOCAL"
cp "$RC_LOCAL" "$TMP/rc.original"

# Old installation state that must survive two upgrades.
printf '%s\n' old > "$ROOT/version-marker"
printf '%s\n' '{"nickname":"old"}' > "$ROOT/config.json"
printf '%s\n' 'persistent data' > "$ROOT/data/persist-marker"
printf '%s\n' 'plugin data' > "$ROOT/sdcard/plugin-marker"
printf '%s\n' 'fake loop image' > "$ROOT/sdcard.img"
printf '%s\n' 'optional-tool' > "$ROOT/ttyd"
cat > "$ROOT/ufi-tools-u60pro" <<'EOF'
#!/bin/sh
trap 'echo stopped > "$UFI_DATA_DIR/stop-marker"; exit 0' TERM
: > "$UFI_DATA_DIR/ready-marker"
while :; do sleep 1; done
EOF
chmod 0755 "$ROOT/ufi-tools-u60pro"
UFI_DATA_DIR="$ROOT/data" "$ROOT/ufi-tools-u60pro" &
OLD_PID=$!
echo "$OLD_PID" > "$ROOT/ufi-tools-u60pro.pid"
i=0
while [ ! -f "$ROOT/data/ready-marker" ] && [ "$i" -lt 50 ]; do
  i=$((i + 1))
  sleep 0.1
done
assert_present "$ROOT/data/ready-marker" "old service did not install its TERM handler"

cp "$SERVICE" "$BUNDLE/service.sh"
cat > "$BUNDLE/ufi-tools-u60pro" <<'EOF'
#!/bin/sh
env | sort > "$UFI_DATA_DIR/runtime-env"
trap 'echo candidate-stopped > "$UFI_DATA_DIR/candidate-stop-marker"; exit 0' TERM
while :; do sleep 1; done
EOF
printf '%s\n' '#!/bin/sh' 'exit 0' > "$BUNDLE/verify-release"
printf '%s\n' '{}' > "$BUNDLE/manifest.json"
printf '%s\n' 'test-key' > "$BUNDLE/public-key.hex"
cat > "$TOOLS/wget" <<'EOF'
#!/bin/sh
printf '{"code":0,"data":{"ok":true,"instance":"%s"}}\n' "${FAKE_HEALTH_INSTANCE:-$(cat "$UFI_INSTALL_ROOT/.ufi-health-instance" 2>/dev/null)}"
exit "${FAKE_WGET_STATUS:-0}"
EOF
chmod 0755 "$BUNDLE/ufi-tools-u60pro" "$BUNDLE/verify-release" "$BUNDLE/service.sh" "$TOOLS/wget"

# First upgrade: data must be copied only after the old process has exited,
# and the boot entry must be created on a device without an rc.local yet.
run_install || fail "first install failed"
if kill -0 "$OLD_PID" 2>/dev/null; then
  fail "legacy service was not stopped"
fi
assert_present "$ROOT/data/stop-marker" "install copied data before the old service exited"
assert_eq "$(cat "$ROOT/config.json")" '{"nickname":"old"}' "config.json was not preserved"
assert_eq "$(cat "$ROOT/data/persist-marker")" 'persistent data' "data marker was not preserved"
assert_eq "$(cat "$ROOT/sdcard/plugin-marker")" 'plugin data' "sdcard data was not preserved"
assert_eq "$(cat "$ROOT/sdcard.img")" 'fake loop image' "sdcard.img was not preserved"
assert_eq "$(cat "$ROOT/ttyd")" 'optional-tool' "optional helper was not preserved"
grep -q '^UFI_DEVICE_BACKEND=u60-local$' "$ROOT/data/runtime-env" || fail "candidate did not start with the U60 backend"
grep -q '^WORKERS_ENABLED=false$' "$ROOT/.ufi-runtime" || fail "worker default was not persisted"
grep -q '^PRIVILEGED_COMPAT=false$' "$ROOT/.ufi-runtime" || fail "write default was not persisted"
assert_present "$RC_LOCAL" "install did not create the boot entry file"
assert_eq "$(rc_count)" 1 "install did not add exactly one boot entry"
assert_present "$ROOT/.ufi-tools-manifest" "install did not write an ownership inventory"
grep -qx 'data' "$ROOT/.ufi-tools-manifest" || fail "inventory is missing data/"
grep -qx 'config.json' "$ROOT/.ufi-tools-manifest" || fail "inventory is missing config.json"
grep -qx 'ufi-tools-u60pro' "$ROOT/.ufi-tools-manifest" || fail "inventory is missing the binary"
[ ! -e "$ROOT/.ufi-tools-pending" ] || fail "successful install left a pending marker"
printf '%s\n' '# local admin line' >> "$RC_LOCAL"
# The first insertion must precede the existing system exit statement.

# Second upgrade: the active tree, including data changed after the first
# upgrade, must be migrated again without duplicating the boot entry.
printf '%s\n' 'updated data' > "$ROOT/data/persist-marker"
printf '%s\n' 'updated plugin' > "$ROOT/sdcard/plugin-marker"
mkdir -p "$ROOT/plugin-store"
printf '%s\n' 'local catalog' > "$ROOT/plugin-store/catalog.json"
run_install || fail "second install failed"
assert_eq "$(cat "$ROOT/config.json")" '{"nickname":"old"}' "config.json was lost on the second upgrade"
assert_eq "$(cat "$ROOT/data/persist-marker")" 'updated data' "data marker was lost on the second upgrade"
assert_eq "$(cat "$ROOT/sdcard/plugin-marker")" 'updated plugin' "sdcard data was lost on the second upgrade"
assert_eq "$(cat "$ROOT/sdcard.img")" 'fake loop image' "sdcard.img was lost on the second upgrade"
assert_eq "$(cat "$ROOT/plugin-store/catalog.json")" 'local catalog' "plugin store data was lost on the second upgrade"
assert_present "$ROOT/data/candidate-stop-marker" "second install did not wait for the candidate to exit"
assert_eq "$(rc_count)" 1 "second install duplicated the boot entry"
awk -v line="$RC_LINE" '$0 == line {seen=1} /^exit 0$/ {exit !seen}' "$RC_LOCAL" || fail "boot entry is unreachable after exit 0"
[ ! -e "$ROOT/.ufi-tools-pending" ] || fail "second install left a pending marker"
grep -Fqx '# local admin line' "$RC_LOCAL" || fail "install overwrote unrelated rc.local content"

# A live process alone is insufficient: another healthy instance on the port
# must not cause the installer to accept the candidate.
export FAKE_HEALTH_INSTANCE=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
if run_install; then fail "accepted another process's health response"; fi
unset FAKE_HEALTH_INSTANCE
assert_eq "$(cat "$ROOT/data/persist-marker")" 'updated data' "wrong-instance rollback lost data"

old_pid=$(cat "$ROOT/ufi-tools.pid")
printf '#!/bin/sh\nexit 9\n' > "$BUNDLE/verify-release"
if run_install; then fail "accepted verifier rejection"; fi
assert_eq "$(cat "$ROOT/ufi-tools.pid")" "$old_pid" "verification failure restarted active service"
kill -0 "$old_pid" 2>/dev/null || fail "verification failure stopped active service"
printf '#!/bin/sh\nexit 0\n' > "$BUNDLE/verify-release"

# A candidate that exits immediately must fail even when an unrelated health
# endpoint answers successfully. The previous tree must be restored.
printf '%s\n' 'rollback-source' > "$ROOT/version-marker"
cat > "$BUNDLE/ufi-tools-u60pro" <<'EOF'
#!/bin/sh
exit 23
EOF
chmod 0755 "$BUNDLE/ufi-tools-u60pro"
if run_install; then
  fail "install accepted a candidate that exited with status 23"
fi
assert_eq "$(cat "$ROOT/version-marker")" 'rollback-source' "rollback did not restore the previous tree"
assert_eq "$(cat "$ROOT/config.json")" '{"nickname":"old"}' "rollback lost config.json"
assert_eq "$(cat "$ROOT/data/persist-marker")" 'updated data' "rollback lost the data marker"
assert_present "$ROOT/ufi-tools.pid" "rollback did not restart the previous service"
ROLLBACK_PID=$(cat "$ROOT/ufi-tools.pid")
kill -0 "$ROLLBACK_PID" 2>/dev/null || fail "the restored service is not running"
assert_eq "$(rc_count)" 1 "failed install changed the boot entry"

# Uninstall must stop the service and auxiliary processes, remove only our
# files and boot entry, and leave everything else alone. It must also be safe
# to run twice.
printf '%s\n' '#!/bin/sh' 'while :; do sleep 1; done' > "$ROOT/ttyd"
chmod 0755 "$ROOT/ttyd"
"$ROOT/ttyd" &
TTYD_PID=$!
echo "$TTYD_PID" > "$ROOT/data/ttyd.pid"
printf '%s\n' keep > "$TMP/keep.txt"
run_service "$ROOT/service.sh" uninstall || fail "uninstall failed"
kill -0 "$ROLLBACK_PID" 2>/dev/null && fail "uninstall left the service running"
kill -0 "$TTYD_PID" 2>/dev/null && fail "uninstall left the auxiliary process running"
[ ! -e "$ROOT" ] || fail "uninstall left the install root behind"
assert_present "$ROOT.uninstalled/data/persist-marker" "default uninstall discarded current data"
[ ! -e "$STAGING" ] || fail "uninstall left the staging directory behind"
assert_present "$TMP/keep.txt" "uninstall touched an unrelated file"
assert_present "$RC_LOCAL" "uninstall removed rc.local"
assert_eq "$(rc_count)" 0 "uninstall left the boot entry behind"
grep -Fqx '# local admin line' "$RC_LOCAL" || fail "uninstall removed unrelated rc.local content"
run_service "$SERVICE" uninstall || fail "uninstall is not idempotent"

# Side-by-side: a second tree on another port, without autostart, must leave
# the original tree and process untouched.
mkdir -p "$SIDE_ROOT" "$SIDE_BUNDLE" "$SIDE_STAGING" "$ORIGINAL_ROOT"
printf '%s\n' "$SIDE_ROOT" > "$SIDE_STAGING.owner"
printf '%s\n' '#!/bin/sh' 'while :; do sleep 1; done' > "$ORIGINAL_ROOT/ufi-tools-u60pro"
chmod 0755 "$ORIGINAL_ROOT/ufi-tools-u60pro"
"$ORIGINAL_ROOT/ufi-tools-u60pro" &
ORIGINAL_PID=$!
cp "$SERVICE" "$SIDE_BUNDLE/service.sh"
cp "$BUNDLE/verify-release" "$SIDE_BUNDLE/verify-release"
cp "$BUNDLE/manifest.json" "$SIDE_BUNDLE/manifest.json"
cp "$BUNDLE/public-key.hex" "$SIDE_BUNDLE/public-key.hex"
cat > "$SIDE_BUNDLE/ufi-tools-u60pro" <<'EOF'
#!/bin/sh
env | sort > "$UFI_DATA_DIR/runtime-env"
trap 'echo side-stopped > "$UFI_DATA_DIR/side-stop-marker"; exit 0' TERM
while :; do sleep 1; done
EOF
chmod 0755 "$SIDE_BUNDLE/ufi-tools-u60pro" "$SIDE_BUNDLE/verify-release" "$SIDE_BUNDLE/service.sh"

run_side "$SIDE_BUNDLE/service.sh" install "$SIDE_BUNDLE" || fail "side-by-side install failed"
kill -0 "$ORIGINAL_PID" 2>/dev/null || fail "side-by-side install stopped the original process"
assert_present "$SIDE_ROOT/data/runtime-env" "side-by-side candidate did not start"
# Opt-ins are explicit persistent operator settings, never hard-forced off.
sed -i.bak 's/^WORKERS_ENABLED=false$/WORKERS_ENABLED=true/;s/^PRIVILEGED_COMPAT=false$/PRIVILEGED_COMPAT=true/;s/^AT_ENABLED=false$/AT_ENABLED=true/' "$SIDE_ROOT/.ufi-runtime"
run_side "$SIDE_ROOT/service.sh" restart || fail "runtime opt-in restart failed"
i=0
while ! grep -qx 'UFI_WORKERS_ENABLED=true' "$SIDE_ROOT/data/runtime-env" && [ "$i" -lt 50 ]; do
  i=$((i+1))
  sleep 0.1
done

grep -qx 'UFI_WORKERS_ENABLED=true' "$SIDE_ROOT/data/runtime-env" || fail "worker opt-in was ignored"
grep -qx 'UFI_PRIVILEGED_COMPAT=true' "$SIDE_ROOT/data/runtime-env" || fail "write opt-in was ignored"
grep -qx 'UFI_AT_ENABLED=true' "$SIDE_ROOT/data/runtime-env" || fail "AT opt-in was ignored"

assert_present "$SIDE_ROOT/.ufi-tools-manifest" "side-by-side install did not write an inventory"
[ ! -e "$SIDE_ROOT/.ufi-tools-pending" ] || fail "side-by-side install left a pending marker"
grep -qx 'UFI_HTTP_ADDR=:2433' "$SIDE_ROOT/data/runtime-env" || fail "candidate ignored the alternate port"
grep -qx 'UFI_HOSTS_PATH=' "$SIDE_ROOT/data/runtime-env" || fail "candidate did not disable hosts management"
grep -qx "UFI_TTYD_BINARY=$SIDE_ROOT/ttyd" "$SIDE_ROOT/data/runtime-env" || fail "candidate ttyd path is not isolated"
grep -qx 'UFI_TTYD_PORT=1147' "$SIDE_ROOT/data/runtime-env" || fail "candidate ttyd port is not isolated"
grep -qx "UFI_SPEEDTEST_BINARY=$SIDE_ROOT/speedtest" "$SIDE_ROOT/data/runtime-env" || fail "candidate speedtest path is not isolated"
grep -qx "UFI_AT_BINARY=$SIDE_ROOT/sendat" "$SIDE_ROOT/data/runtime-env" || fail "candidate AT path is not isolated"
[ ! -e "$SIDE_ROOT.rc.local" ] || fail "no-autostart install created a boot entry"

# keep-data: stop and remove the boot entry, but keep the data for a reinstall.
run_side "$SIDE_ROOT/service.sh" uninstall --keep-data || fail "keep-data uninstall failed"
[ -d "$SIDE_ROOT" ] || fail "keep-data uninstall removed the install root"
assert_present "$SIDE_ROOT/data/runtime-env" "keep-data uninstall removed candidate data"
if [ -f "$SIDE_ROOT/ufi-tools.pid" ]; then
  kill -0 "$(cat "$SIDE_ROOT/ufi-tools.pid")" 2>/dev/null && fail "keep-data left the service running"
fi
[ ! -e "$SIDE_STAGING" ] || fail "keep-data uninstall left its staging behind"
[ ! -e "$SIDE_ROOT.rc.local" ] || fail "keep-data uninstall created or kept a boot entry"

# Reinstall over the kept data, then default uninstall keeps .previous.
run_side "$SIDE_BUNDLE/service.sh" install "$SIDE_BUNDLE" || fail "reinstall over kept data failed"
run_side "$SIDE_ROOT/service.sh" uninstall || fail "keep-backup uninstall failed"
[ ! -e "$SIDE_ROOT" ] || fail "keep-backup uninstall left the active root behind"
[ -d "$SIDE_ROOT.previous" ] || fail "keep-backup uninstall did not keep the previous tree"
[ ! -e "$SIDE_STAGING" ] || fail "keep-backup uninstall left its staging behind"

# restore: put the previous tree back and start it.
run_side "$SIDE_BUNDLE/service.sh" uninstall --restore || fail "restore uninstall failed"
[ -d "$SIDE_ROOT" ] || fail "restore uninstall did not restore the previous tree"
[ ! -e "$SIDE_ROOT.previous" ] || fail "restore uninstall left the previous tree behind"
assert_present "$SIDE_ROOT/data/runtime-env" "restore uninstall lost the restored data"
[ -f "$SIDE_ROOT/ufi-tools.pid" ] || fail "restore uninstall did not start the restored service"
kill -0 "$(cat "$SIDE_ROOT/ufi-tools.pid")" 2>/dev/null || fail "restored service is not running"
[ ! -e "$SIDE_STAGING" ] || fail "restore uninstall left its staging behind"

# purge: explicitly remove active, previous and staging.
mkdir -p "$SIDE_ROOT.previous"
printf '%s\n' '# ufi-tools managed installation' > "$SIDE_ROOT.previous/.ufi-tools-manifest"
printf '%s\n' previous > "$SIDE_ROOT.previous/marker"
run_side "$SIDE_ROOT/service.sh" uninstall --purge || fail "purge uninstall failed"
[ ! -e "$SIDE_ROOT" ] || fail "purge uninstall left the active root behind"
[ ! -e "$SIDE_ROOT.previous" ] || fail "purge uninstall left the previous tree behind"
[ ! -e "$SIDE_ROOT.uninstalled" ] || fail "purge left the saved uninstall behind"
[ ! -e "$SIDE_STAGING" ] || fail "purge uninstall left its staging behind"
[ ! -e "$SIDE_ROOT.rc.local" ] || fail "purge uninstall left a boot entry"

# Reinstall after a full uninstall, then clean up.
run_side "$SIDE_BUNDLE/service.sh" install "$SIDE_BUNDLE" || fail "reinstall after purge failed"
[ -x "$SIDE_ROOT/ufi-tools-u60pro" ] || fail "reinstall after purge did not install the binary"
run_side "$SIDE_ROOT/service.sh" uninstall || fail "final side-by-side uninstall failed"
[ ! -e "$SIDE_ROOT" ] || fail "final side-by-side uninstall left its root behind"
kill -0 "$ORIGINAL_PID" 2>/dev/null || fail "side-by-side uninstall stopped the original process"
kill "$ORIGINAL_PID" 2>/dev/null || true
unset ORIGINAL_PID

# Failure injection: simulated copy/disk-full, failed rotation and SIGTERM
# after rotation must all restore the previous tree and service.
mkdir -p "$FAIL_TOOLS"
cp -R "$SIDE_BUNDLE" "$FAIL_BUNDLE"
cat > "$FAIL_TOOLS/cp" <<'EOF'
#!/bin/sh
if [ -n "${FAIL_CP_MATCH:-}" ]; then
  case "$*" in
    *"$FAIL_CP_MATCH"*) exit 1 ;;
  esac
fi
exec /bin/cp "$@"
EOF
cat > "$FAIL_TOOLS/mv" <<'EOF'
#!/bin/sh
if [ -n "${FAIL_MV_MATCH:-}" ]; then
  case "$*" in
    *"$FAIL_MV_MATCH"*) exit 1 ;;
  esac
fi
/bin/mv "$@"
status=$?
if [ -n "${FAIL_SIGNAL_MATCH:-}" ]; then
  case "$*" in
    *"$FAIL_SIGNAL_MATCH"*) kill -TERM "$PPID" ;;
  esac
fi
exit "$status"
EOF
chmod 0755 "$FAIL_TOOLS/cp" "$FAIL_TOOLS/mv"

reset_fail_root
export FAIL_CP_MATCH="$FAIL_ROOT/data/."
if run_fail "$FAIL_BUNDLE/service.sh" install "$FAIL_BUNDLE"; then
  fail "install succeeded despite a simulated disk-full copy failure"
fi
unset FAIL_CP_MATCH
assert_fail_root_restored "copy failure"
assert_eq "$(cat "$FAIL_ROOT/version-marker")" fail-old "copy failure lost the version marker"

reset_fail_root
export FAIL_MV_MATCH="$FAIL_ROOT.next $FAIL_ROOT"
if run_fail "$FAIL_BUNDLE/service.sh" install "$FAIL_BUNDLE"; then
  fail "install succeeded despite a failed directory rotation"
fi
unset FAIL_MV_MATCH
assert_fail_root_restored "rotation failure"
assert_eq "$(cat "$FAIL_ROOT/config.json")" fail-old "rotation failure lost config.json"

reset_fail_root
export FAIL_SIGNAL_MATCH="$FAIL_ROOT.next $FAIL_ROOT"
if run_fail "$FAIL_BUNDLE/service.sh" install "$FAIL_BUNDLE"; then
  fail "install succeeded despite SIGTERM after rotation"
fi
unset FAIL_SIGNAL_MATCH
assert_fail_root_restored "SIGTERM after rotation"

# Power-loss style recovery: no ROOT, an old BACKUP and a stale NEXT.
reset_fail_root
run_fail "$FAIL_BUNDLE/service.sh" stop || fail "could not stop the failure fixture"
mv "$FAIL_ROOT" "$FAIL_ROOT.previous"
mkdir -p "$FAIL_ROOT.next"
printf '%s\n' '# ufi-tools managed installation' > "$FAIL_ROOT.next/.ufi-tools-manifest"
printf '%s\n' stale > "$FAIL_ROOT.next/stale"
run_fail "$FAIL_BUNDLE/service.sh" install "$FAIL_BUNDLE" || fail "install did not recover an interrupted rotation"
assert_eq "$(cat "$FAIL_ROOT/data/persist-marker")" fail-old "recovery install lost the data marker"
assert_present "$FAIL_ROOT.previous" "recovery install did not keep the restored old tree as backup"
assert_present "$FAIL_ROOT/ufi-tools.pid" "recovery install did not start the candidate"
kill -0 "$(cat "$FAIL_ROOT/ufi-tools.pid")" 2>/dev/null || fail "recovered candidate is not running"

# An ambiguous uninstall must refuse; --restore must recover the tree.
reset_fail_root
run_fail "$FAIL_BUNDLE/service.sh" stop || fail "could not stop the failure fixture for restore"
mv "$FAIL_ROOT" "$FAIL_ROOT.previous"
if run_fail "$FAIL_BUNDLE/service.sh" uninstall; then
  fail "uninstall deleted an ambiguous interrupted tree"
fi
run_fail "$FAIL_BUNDLE/service.sh" uninstall --restore || fail "uninstall --restore failed"
assert_present "$FAIL_ROOT" "uninstall --restore did not restore the tree"
assert_eq "$(cat "$FAIL_ROOT/data/persist-marker" 2>/dev/null || true)" fail-old "restore lost the data marker"
assert_present "$FAIL_ROOT/ufi-tools.pid" "uninstall --restore did not start the service"
kill -0 "$(cat "$FAIL_ROOT/ufi-tools.pid")" 2>/dev/null || fail "restored service is not running"
# A pending marker left after rotation (power loss before health) must roll
# back to the previous tree on the next start.
reset_fail_root
run_fail "$FAIL_BUNDLE/service.sh" install "$FAIL_BUNDLE" || fail "pending-marker fixture install failed"
assert_present "$FAIL_ROOT/data/runtime-env" "pending-marker fixture candidate did not start"
printf '%s\n' pending > "$FAIL_ROOT/.ufi-tools-pending"
run_fail "$FAIL_BUNDLE/service.sh" start || fail "start did not roll back an unverified installation"
[ ! -e "$FAIL_ROOT/data/runtime-env" ] || fail "pending rollback kept the unverified candidate"
[ ! -e "$FAIL_ROOT.previous" ] || fail "pending rollback left the previous tree behind"
assert_eq "$(cat "$FAIL_ROOT/data/persist-marker" 2>/dev/null || true)" fail-old "pending rollback lost the data marker"
assert_present "$FAIL_ROOT/ufi-tools.pid" "pending rollback did not start the previous service"
kill -0 "$(cat "$FAIL_ROOT/ufi-tools.pid")" 2>/dev/null || fail "pending-rollback service is not running"
assert_present "$FAIL_ROOT/ufi-tools-u60pro" "pending rollback lost the previous binary"
reset_fail_root

# start must also recover a tree left without an active ROOT.
reset_fail_root
run_fail "$FAIL_BUNDLE/service.sh" stop || fail "could not stop the failure fixture for start recovery"
mv "$FAIL_ROOT" "$FAIL_ROOT.previous"
run_fail "$FAIL_BUNDLE/service.sh" start || fail "start did not recover an interrupted rotation"
assert_eq "$(cat "$FAIL_ROOT/data/persist-marker" 2>/dev/null || true)" fail-old "start recovery lost the data marker"
assert_present "$FAIL_ROOT/ufi-tools.pid" "start recovery did not start the service"
kill -0 "$(cat "$FAIL_ROOT/ufi-tools.pid")" 2>/dev/null || fail "start-recovered service is not running"
reset_fail_root

echo "transaction, preservation, rollback, uninstall, side-by-side and failure-injection tests passed"
