#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
TMP=${TMPDIR:-/tmp}/ufi-tools-adb-test.$$
trap 'rm -rf "$TMP"' EXIT HUP INT TERM
mkdir -p "$TMP/bundle"

for file in manifest.json ufi-tools-u60pro verify-release public-key.hex service.sh SHA256SUMS; do
  : > "$TMP/bundle/$file"
done

cat > "$TMP/adb" <<'EOF'
#!/bin/sh
[ -z "${FAKE_ADB_LOG:-}" ] || printf '%s\n' "$*" >> "$FAKE_ADB_LOG"
if [ "${1:-}" = -s ]; then
  shift 2
fi
# Decode the installer's checked shell transport without executing target paths.
if [ "$1" = shell ]; then
  case "$2" in
    "sh -c "*)
      payload=$(python3 -c 'import shlex,sys; print(shlex.split(sys.argv[1])[2])' "$2")
      marker=$(printf '%s' "$2" | sed -n 's/.*\(__UFI_ADB_EXIT_[0-9]*\).*/\1/p')
      "$0" shell "$payload"
      remote_status=$?
      printf '\n%s=%s\n' "$marker" "$remote_status"
      exit 0 # Model the real legacy adbd: host status alone is always successful.
      ;;
  esac
fi
case "${1:-}" in
  get-state)
    case "${FAKE_ADB_STATE:-}" in
      none|multiple) exit 1 ;;
      *) printf '%s\n' "${FAKE_ADB_STATE:-device}" ;;
    esac
    ;;
  devices)
    printf 'List of devices attached\n'
    case "${FAKE_ADB_STATE:-}" in
      none) ;;
      multiple) printf 'u60\tdevice\nother\tdevice\n' ;;
      *) printf 'u60\t%s\n' "${FAKE_ADB_STATE:-device}" ;;
    esac
    ;;
  shell)
    shift
    case "${1:-}" in
      id*)
        printf '%s\n' "${FAKE_ADB_UID:-0}"
        ;;
      df*)
        case "$1" in
          *awk*) printf '%s\n' "${FAKE_ADB_FREE_KB:-1048576}" ;;
          *)
            printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
            printf '/dev/block/data 1048576 1 %s 1%% /data\n' "${FAKE_ADB_FREE_KB:-1048576}"
            ;;
        esac
        ;;
      \[*)
        case "$1" in
          "[ -w "*|"[ -w "*) exit "${FAKE_ADB_WRITABLE_STATUS:-0}" ;;
          "[ -e "*|"[ -e "*) exit "${FAKE_ADB_ROOT_EXISTS_STATUS:-1}" ;;
          "[ -f "*|"[ -f "*) exit "${FAKE_ADB_ROOT_OWNED_STATUS:-0}" ;;
          "[ -x "*|"[ -x "*) exit "${FAKE_ADB_EXECUTABLE_STATUS:-0}" ;;
          *) exit 0 ;;
        esac
        ;;
      grep*) exit "${FAKE_ADB_ROOT_OWNED_STATUS:-0}" ;;
      "if [ -d "*) printf '%s\n' 0 ;;
      uname*) printf 'aarch64\n' ;;
    esac
    ;;
  push) ;;
  *) exit 2 ;;
esac
EOF
chmod 0755 "$TMP/adb"

expect_failure() {
  state=$1
  expected=$2
  output=$(
    FAKE_ADB_STATE=$state ADB="$TMP/adb" \
      "$ROOT/installer/install-adb.sh" --execute --bundle "$TMP/bundle" 2>&1
  ) && {
    echo "expected failure for ADB state $state" >&2
    exit 1
  }
  printf '%s\n' "$output" | grep -F "$expected" >/dev/null || {
    echo "missing diagnostic for ADB state $state" >&2
    printf '%s\n' "$output" >&2
    exit 1
  }
}

expect_preflight_failure() {
  name=$1
  expected=$2
  shift 2
  output=$(
    env "$@" ADB="$TMP/adb" \
      "$ROOT/installer/install-adb.sh" --execute --serial u60 --bundle "$TMP/bundle" 2>&1
  ) && {
    echo "expected preflight failure for $name" >&2
    exit 1
  }
  printf '%s\n' "$output" | grep -F "$expected" >/dev/null || {
    echo "missing preflight diagnostic for $name" >&2
    printf '%s\n' "$output" >&2
    exit 1
  }
}

expect_failure unauthorized "connected but not authorized"
expect_failure offline "ADB device is offline"
expect_failure none "no ADB interface detected"
expect_failure multiple "multiple ADB devices are connected"

expect_preflight_failure uid "must run as root" FAKE_ADB_UID=1000
expect_preflight_failure free-space "insufficient free space" FAKE_ADB_FREE_KB=1024
expect_preflight_failure writable "is not writable" FAKE_ADB_WRITABLE_STATUS=1
expect_preflight_failure owned "unowned directory" FAKE_ADB_ROOT_EXISTS_STATUS=0 FAKE_ADB_ROOT_OWNED_STATUS=1

FAKE_ADB_STATE=device ADB="$TMP/adb" \
  "$ROOT/installer/install-adb.sh" --execute --serial u60 --bundle "$TMP/bundle" >/dev/null

ADB="$TMP/adb" "$ROOT/installer/install-adb.sh" --execute --serial u60 \
  --uninstall --root /data/ufi-tools-candidate --no-autostart --keep-data >/dev/null

output=$(
  "$ROOT/installer/install-adb.sh" --uninstall --root /data/ufi-tools-candidate \
    --no-autostart --purge 2>&1
)
printf '%s\n' "$output" | grep -F 'uninstall mode: purge' >/dev/null || {
  echo "missing purge uninstall mode in dry-run output" >&2
  printf '%s\n' "$output" >&2
  exit 1
}

# Restore must use the current controller even if the active service is absent,
# and verify that the restored autostart entry exists (not that it was removed).
FAKE_ADB_LOG="$TMP/restore-log" ADB="$TMP/adb" \
  "$ROOT/installer/install-adb.sh" --execute --uninstall --restore --yes >/dev/null
grep -F '.staging.controller' "$TMP/restore-log" >/dev/null || exit 1
grep -F "uninstall --restore" "$TMP/restore-log" >/dev/null || exit 1



for unsafe in '/data/test$(touch /tmp/not-allowed)' '/data/test&echo' '/data/test|cat'; do
  if "$ROOT/installer/install-adb.sh" --bundle "$TMP/bundle" --root "$unsafe" --no-autostart >/dev/null 2>&1; then
    echo 'unsafe remote path was accepted' >&2; exit 1
  fi
done
if "$ROOT/installer/install-adb.sh" --bundle "$TMP/bundle" --staging /data/ufi-tools/tmp >/dev/null 2>&1; then
  echo 'nested staging was accepted' >&2; exit 1
fi
echo "ADB installer state, preflight, restore dispatch and path tests passed"
