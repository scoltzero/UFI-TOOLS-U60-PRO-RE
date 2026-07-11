# LAN backup/restore transport

This is the second installation transport for devices where an authorized ADB
bootstrap is unavailable. Analysis of the supplied installer established the
inner/outer archive names, MD5 sidecar, target paths, multipart upload endpoint,
restore command, and OpenSSL-compatible 3DES function. `cmd/build-lan-backup`
generates a container entirely offline. The password derivation, SHA-256 EVP
key derivation, encrypted envelope, outer archive pair, MD5 sidecar, regular
files, and contained relative symlinks have been validated against a backup
created by the authorized U60 Pro. Restore acceptance still requires a real
reboot, so generation retains an explicit risk acknowledgement.

The generator also requires an `rc.local` captured from the exact matching
firmware. It never invents or silently overwrites this boot-critical file:

```sh
go run ./cmd/build-lan-backup \
  --binary bin/ufi-tools-u60pro-linux-arm64 \
  --service installer/bundle/service.sh \
  --rc-local /authorized-fixture/rc.local \
  --imei-file /secure/u60-imei \
  --output build/backup_restore.bin \
  --acknowledge-unverified-format
```

This command does not open a network connection. Preview the resulting LAN
transaction without contacting the device:

```sh
go run ./cmd/install-lan \
  --base http://192.168.98.1 \
  --container build/backup_restore.bin \
  --rollback-container build/closed-rollback.bin
```

The explicit execution form requires a separately verified rollback container
that restores the currently installed service, service script, and `rc.local`.
A normal vendor parameter backup is not sufficient because it does not prove
that custom files under `/data/ufi-tools` will be restored. The installer saves
a local mode-`0600` safety copy, uploads the candidate to
`/cgi-bin/cgi-upload`, and calls `device_restore_proc`. Success requires a real
health transition from unavailable back to healthy. On candidate failure it
uploads the rollback container, triggers a second restore, and also requires
the rollback service to complete a down/up health transition:

```sh
go run ./cmd/install-lan \
  --base http://192.168.98.1 \
  --container build/backup_restore.bin \
  --rollback-container build/closed-rollback.bin \
  --backup-output build/rollback-safety-copy.bin \
  --router-password-file /secure/router-web-password \
  --execute --acknowledge-device-restore
```

Inspect an authorized vendor backup without extracting it to disk:

```sh
go run ./cmd/inspect-zte-backup \
  --input /secure/back_parameter \
  --imei-file /secure/u60-imei
```

The device-bound 3DES password is derived from the matching 15-digit IMEI and
the firmware suffix recovered from the supplied installer. Key and IV material
use OpenSSL-compatible SHA-256 EVP derivation. Reading the IMEI from a file
keeps it out of command-line arguments and Git history.

The installer reproduces the official login sequence recovered from both the
public web client and supplied installer: `web_login_info`, two uppercase
SHA-256 passes with `zte_web_sault`, `web_login`, and the returned
`ubus_rpc_session`. The password is read from a file and never logged. An
already-authorized 32-character session may instead be passed with
`--ubus-session`; the two options are mutually exclusive. Authenticated CGI
uploads include the firmware's `webtoken` cookie and browser headers.

The client rejects public targets, redirects, malformed sessions, oversized
responses, and upload responses that do not contain both checksum
confirmations. On the authorized device, anonymous UBUS returns access denied
and an anonymous CGI upload returns HTTP 405, confirming that login is required
before the remaining restore test.

Validated acceptance criteria:

- decrypt and inspect a user-owned real-device backup;
- parse and regenerate it deterministically without proprietary Qualcomm tools;
- validate path safety, archive type, envelope, and checksum fields;
- produce an inspection manifest showing every modified path;
- generate both candidate and matching closed-service rollback containers.

The remaining acceptance criterion is a successful candidate restore, reboot,
health recovery, deliberately exercised rollback, and post-rollback baseline
comparison on the authorized device.

The payload and transaction must remain identical to the ADB bundle. Only the
transport/container adapter differs.
