# U60 Pro validation record

This document contains sanitized, non-secret facts gathered from the
user-owned test device. Raw configuration, binaries and responses remain under
the Git-ignored `research-inputs/` directory.

## Authorized UBUS calibration (2026-07-11)

The connected U60 Pro exposes its official management API through OpenWrt
`uhttpd` `/ubus`, not the older ZTE `/goform` CGI transport. The dedicated
`u60-ubus` backend was calibrated against the device's public web assets and
live read-only responses for device manager, network information, SIM state,
and router status.

A static ARM64 candidate ran from `/tmp` on loopback port 2433 without replacing
the installed service or changing boot configuration. Health, the complete
embedded UI, device/version summaries, and legacy multi-key reads all returned
HTTP 200 through an ADB forward. The live response reported SA network type and
signal level 4. Device identifiers and the temporary candidate token were not
recorded.

U60 Pro deployments should set `UFI_DEVICE_BACKEND=u60-ubus` and
`UFI_DEVICE_BASE_URL=http://127.0.0.1`. A logged-in 32-character UBUS session is
optional; the anonymous all-zero session supports the calibrated read-only
procedures.

## Read-only baseline

- Kernel: OpenWrt Linux 5.15, AArch64.
- The reference service is installed at `/data/ufi-tools` and listens on 2333.
- `/etc/rc.local` starts `/data/ufi-tools/service.sh`.
- ttyd listens on 1146 and network ADB listens on 5555.
- The reference service exposes a health endpoint and requires login-token
  authentication for protected compatibility routes.
- No file was written, no process was restarted, and no setting was changed
  during baseline collection.

## Fixture collection

`cmd/compat-probe` performs allowlisted GET requests only, rejects public
targets, limits response sizes, and recursively removes tokens, passwords,
cookies and device identifiers. A typical USB-forwarded capture is:

```sh
adb -s SERIAL forward tcp:22333 tcp:2333
go run ./cmd/compat-probe \
  --base http://127.0.0.1:22333 \
  --token-file /secure/token \
  --output research-inputs/device/compat-fixture.json
adb -s SERIAL forward --remove tcp:22333
```

The current device has login-token authentication enabled. A future fixture
capture needs the administrator's plaintext web token; the stored hash is not
reversible and is deliberately not bypassed.

## Mutation gate

Before the first write test, capture the current service tree, `rc.local`, the
vendor backup container, hashes, free space and active process/port state. Each
mutation test must name its rollback command and verify the baseline afterward.
