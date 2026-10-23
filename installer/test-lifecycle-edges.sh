#!/bin/sh
set -eu
PROJECT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
SERVICE=$PROJECT/installer/bundle/service.sh
TMP=$(mktemp -d)
ROOT=$TMP/root
TOOLS=$TMP/tools
mkdir -p "$TOOLS" "$ROOT/data"
cleanup() {
  if [ -f "$ROOT/ufi-tools-u60pro.pid" ]; then kill "$(cat "$ROOT/ufi-tools-u60pro.pid")" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT HUP INT TERM
fail() { echo "FAIL: $*" >&2; exit 1; }
export UFI_INSTALL_ROOT="$ROOT" UFI_RC_LOCAL="$TMP/rc.local" UFI_PLUGIN_MOUNT="$TMP/sdcard"
export UFI_PLUGIN_STORAGE_ENABLED=false UFI_STAGING_DIR="$TMP/staging"
export UFI_MOUNTINFO="$TMP/mountinfo" UFI_HEALTH_SETTLE_SECONDS=0
export PATH="$TOOLS:$PATH"
if UFI_INSTALL_ROOT=/data/ufi-tools sh "$SERVICE" uninstall --purge; then fail 'accepted original installation root'; fi

# Failure during the atomic boot-file replacement cannot truncate the live file.
mkdir -p "$TMP/atomic-root"
printf '%s\n' '# ufi-tools managed installation' > "$TMP/atomic-root/.ufi-tools-manifest"
printf '# original boot file\nexit 0' > "$TMP/atomic-rc"
cp "$TMP/atomic-rc" "$TMP/atomic-before"
cat > "$TOOLS/mv" <<'EOF'
#!/bin/sh
if [ "${FAIL_BOOT_REPLACE:-0}" = 1 ]; then
  case "$*" in *'.ufi-rc.'*) exit 1 ;; esac
fi
exec /bin/mv "$@"
EOF
chmod 755 "$TOOLS/mv"
if UFI_INSTALL_ROOT="$TMP/atomic-root" UFI_RC_LOCAL="$TMP/atomic-rc" FAIL_BOOT_REPLACE=1 sh "$SERVICE" enable; then fail 'accepted failed boot write'; fi
cmp -s "$TMP/atomic-rc" "$TMP/atomic-before" || fail 'failed write changed original boot file'
UFI_INSTALL_ROOT="$TMP/atomic-root" UFI_RC_LOCAL="$TMP/atomic-rc" sh "$SERVICE" enable
UFI_INSTALL_ROOT="$TMP/atomic-root" UFI_RC_LOCAL="$TMP/atomic-rc" sh "$SERVICE" disable
cmp -s "$TMP/atomic-rc" "$TMP/atomic-before" || fail 'boot file not restored'

printf '#!/bin/sh\nexit 0\n' > "$TOOLS/wget"
chmod 755 "$TOOLS/wget"
: > "$UFI_MOUNTINFO"
printf '#!/bin/sh\nexit 0\n' > "$ROOT/ufi-tools-u60pro"
printf 'important current data\n' > "$ROOT/data/marker"
printf '%s\n' '# ufi-tools managed installation' > "$ROOT/.ufi-tools-manifest"

mkdir "$ROOT.lifecycle-lock"
if sh "$SERVICE" uninstall --purge; then fail 'accepted overlapping transaction'; fi
[ -f "$ROOT/data/marker" ] || fail 'concurrent transaction modified data'
rmdir "$ROOT.lifecycle-lock"

# Default uninstall must retain the CURRENT state even on a first install.
sh "$SERVICE" uninstall
[ "$(cat "$ROOT.uninstalled/data/marker")" = 'important current data' ] || fail 'lost first-install data'

# The restored script, not the new controller, determines legacy boot behavior.
cat > "$ROOT.uninstalled/service.sh" <<'EOF'
#!/bin/sh
printf 'legacy-start-called\n' > "$UFI_INSTALL_ROOT/restored-script-marker"
"$UFI_INSTALL_ROOT/ufi-tools-u60pro" >/dev/null 2>&1 &
echo $! > "$UFI_INSTALL_ROOT/ufi-tools-u60pro.pid"
EOF
printf '#!/bin/sh\ntrap "exit 0" TERM\nwhile :; do sleep 1; done\n' > "$ROOT.uninstalled/ufi-tools-u60pro"
chmod 755 "$ROOT.uninstalled/ufi-tools-u60pro"
printf '#!/bin/sh\n# other startup\nexit 0\n'  > "$UFI_RC_LOCAL"
sh "$SERVICE" uninstall --restore
[ -f "$ROOT/restored-script-marker" ] || fail 'did not use restored service script'
awk -v line="sh $ROOT/service.sh start" '$0 == line {seen=1} /^exit 0$/ {exit !seen}' "$UFI_RC_LOCAL" || fail 'restore startup unreachable'

# A broken previous service must not destroy the currently working tree.
mkdir -p "$ROOT.previous/data"
printf '%s\n' '# ufi-tools managed installation' > "$ROOT.previous/.ufi-tools-manifest"
printf 'broken snapshot\n' > "$ROOT.previous/data/marker"
printf '#!/bin/sh\nexit 7\n' > "$ROOT.previous/service.sh"
: > "$ROOT.previous/ufi-tools-u60pro"
if sh "$SERVICE" uninstall --restore; then fail 'accepted broken restore'; fi
[ "$(cat "$ROOT/data/marker")" = 'important current data' ] || fail 'restore failure lost current data'
[ "$(cat "$ROOT.previous/data/marker")" = 'broken snapshot' ] || fail 'restore failure lost previous snapshot'
[ ! -e "$ROOT.restore-reserve" ] || fail 'failed restore did not put current tree back'
kill -0 "$(cat "$ROOT/ufi-tools-u60pro.pid")" || fail 'current service not recovered'
sh "$SERVICE" stop

# Isolated install never owns or detaches the original /sdcard.
printf '44 1 0:1 /other %s rw - ext4 /dev/test rw\n' "$UFI_PLUGIN_MOUNT" > "$UFI_MOUNTINFO"
sh "$SERVICE" uninstall --purge
[ -s "$UFI_MOUNTINFO" ] || fail 'uninstall touched another service mount'
: > "$UFI_MOUNTINFO"

# No nested filesystem, symlink tree or arbitrary data directory can be removed.
mkdir -p "$ROOT/data"
printf '%s\n' '# ufi-tools managed installation' > "$ROOT/.ufi-tools-manifest"
: > "$ROOT/ufi-tools-u60pro"
printf '43 1 0:1 / %s rw - ext4 /dev/test rw\n' "$ROOT/data" > "$UFI_MOUNTINFO"
if sh "$SERVICE" uninstall --purge; then fail 'accepted nested mount'; fi
[ -d "$ROOT/data" ] || fail 'removed nested mount data'
: > "$UFI_MOUNTINFO"
rm "$ROOT/ufi-tools-u60pro" "$ROOT/.ufi-tools-manifest"
if sh "$SERVICE" uninstall --purge; then fail 'accepted arbitrary data directory'; fi
mv "$ROOT" "$TMP/unrelated"
ln -s "$TMP/unrelated" "$ROOT"
if sh "$SERVICE" uninstall --purge; then fail 'accepted symlink root'; fi
[ -d "$TMP/unrelated/data" ] || fail 'removed unrelated target'
echo 'lifecycle restore, boot ordering, saved data and mount-guard tests passed'
