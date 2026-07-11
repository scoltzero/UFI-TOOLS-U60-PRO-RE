#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TMP=${TMPDIR:-/tmp}/ufi-tools-adb-test.$$
trap 'rm -rf "$TMP"' EXIT HUP INT TERM
mkdir -p "$TMP/bundle"

for file in manifest.json ufi-tools-u60pro verify-release public-key.hex service.sh; do
  : > "$TMP/bundle/$file"
done

cat > "$TMP/adb" <<'EOF'
#!/bin/sh
if [ "${1:-}" = -s ]; then
  shift 2
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

expect_failure unauthorized "connected but not authorized"
expect_failure offline "ADB device is offline"
expect_failure none "no ADB interface detected"
expect_failure multiple "multiple ADB devices are connected"

FAKE_ADB_STATE=device ADB="$TMP/adb" \
  "$ROOT/installer/install-adb.sh" --execute --serial u60 --bundle "$TMP/bundle" >/dev/null

echo "ADB installer state diagnostics passed"
