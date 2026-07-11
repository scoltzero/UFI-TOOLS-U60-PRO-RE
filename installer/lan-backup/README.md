# LAN backup/restore transport

This is the second installation transport for devices where an authorized ADB
bootstrap is unavailable. Analysis of the supplied installer established the
inner/outer archive names, MD5 sidecar, target paths, multipart upload endpoint,
restore command, and OpenSSL-compatible 3DES function. `cmd/build-lan-backup`
therefore generates a container entirely offline, but refuses to run unless the
operator acknowledges that a redacted, known-good U60 Pro backup fixture has not
yet confirmed byte-for-byte compatibility.

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

This command does not open a network connection. Upload to
`/cgi-bin/cgi-upload` and the `device_restore_proc` trigger are deliberately not
implemented until fixture compatibility and rollback have been proven.

Implementation acceptance criteria:

- validate the generated container against user-owned, redacted fixtures;
- parse and regenerate it deterministically without proprietary Qualcomm tools;
- validate all path, length, checksum, and model/version fields;
- produce a preview manifest showing every modified path;
- require an explicit `--execute` confirmation for network restore;
- preserve the original backup and implement an automatic rollback artifact.

The payload and transaction must remain identical to the ADB bundle. Only the
transport/container adapter differs.
