# LAN backup/restore transport

This is the second installation transport for devices where an authorized ADB
bootstrap is unavailable. Analysis of the supplied installer established the
inner/outer archive names, MD5 sidecar, target paths, multipart upload endpoint,
restore command, and OpenSSL-compatible 3DES function. `cmd/build-lan-backup`
generates a container entirely offline. It refuses to run unless the operator
acknowledges that a redacted, known-good U60 Pro backup fixture has not yet
confirmed byte-for-byte compatibility.

The generator also requires an `rc.local` captured from the exact matching
firmware. It never invents or silently overwrites this boot-critical file:

```sh
go run ./cmd/build-lan-backup \
  --binary build/release-0.1.0/ufi-tools-u60pro \
  --service installer/bundle/service.sh \
  --rc-local /authorized-fixture/rc.local \
  --password-file /secure/backup-password \
  --output build/backup_restore.bin \
  --acknowledge-unverified-format
```

This command does not open a network connection. Preview the resulting LAN
transaction without contacting the device:

```sh
go run ./cmd/install-lan \
  --base http://192.168.0.1 \
  --container build/backup_restore.bin
```

The explicit execution form first asks the official U60 web service to create
an original backup, saves it locally with mode `0600`, uploads the candidate to
`/cgi-bin/cgi-upload`, and calls `device_restore_proc`. Success requires a real
health transition from unavailable back to healthy; otherwise the client
retries the original backup upload and triggers a rollback restore:

```sh
go run ./cmd/install-lan \
  --base http://192.168.0.1 \
  --container build/backup_restore.bin \
  --backup-output build/original-back_parameter.bin \
  --execute --acknowledge-device-restore
```

If the official web UI requires a logged-in UBUS session, pass its authorized
32-character value with `--ubus-session`. The client rejects public targets,
redirects, malformed sessions, oversized responses, and upload responses that
do not contain both checksum confirmations.

Implementation acceptance criteria:

- validate the generated container against user-owned, redacted fixtures;
- parse and regenerate it deterministically without proprietary Qualcomm tools;
- validate all path, length, checksum, and model/version fields;
- produce a preview manifest showing every modified path;
- validate the generated container against a user-owned real-device backup
  before treating LAN restore as production-calibrated.

The payload and transaction must remain identical to the ADB bundle. Only the
transport/container adapter differs.
