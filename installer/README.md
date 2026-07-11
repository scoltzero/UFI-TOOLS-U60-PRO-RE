# Installer

Both installation transports consume the same release bundle and transaction:

1. preflight architecture, free space, permissions, and current service;
2. verify the release manifest, Ed25519 signature, digest, and artifact size;
3. stage into `/data/ufi-tools.next` without overwriting the active version;
4. stop the existing service and atomically rotate active/backup directories;
5. install the service entry and start the candidate;
6. check `http://127.0.0.1:2333/healthz`;
7. restore the backup if any step or the health check fails.

`install-adb.sh` is the preferred authorized-ADB transport. It defaults to a
dry run and does not contact a device unless `--execute` is passed. The LAN
backup transport is documented as a bundle adapter and remains disabled until
the public backup container format is implemented and fixture-tested.

The signing private key is never stored in this repository or release bundle.

