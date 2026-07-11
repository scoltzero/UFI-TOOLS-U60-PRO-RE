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
transport has the same dry-run default and implements original-backup capture,
the official multipart upload and UBUS restore calls, restart-aware health
checking, and automatic rollback. The container format still needs final
calibration against an authorized real-device backup before production use.

Build and preview the LAN transport with:

```sh
go run ./cmd/install-lan --base http://192.168.0.1 \
  --container build/backup_restore.bin
```

The signing private key is never stored in this repository or release bundle.

Generate a release keypair once with `go run ./cmd/release-keygen`, move the
private key to offline secret storage, and build a verified bundle with:

```sh
VERSION=0.1.0 PRIVATE_KEY=/secure/private.hex PUBLIC_KEY=release-public-key.hex \
  ./scripts/make-release.sh
```

`installer/test-transaction.sh` exercises the atomic swap and failure rollback
entirely under a temporary directory with fake health and verifier commands.

Optional open-source `ttyd` and speed-test binaries can be included without
vendoring them in this repository:

```sh
TTYD_BINARY=/path/to/linux-arm64/ttyd \
SPEEDTEST_BINARY=/path/to/linux-arm64/speedtest \
VERSION=0.1.0 PRIVATE_KEY=/secure/private.hex PUBLIC_KEY=release-public-key.hex \
  ./scripts/make-release.sh
```

They are preserved across later core-only updates. Their licenses must be
added to the release's third-party notices by the distributor.
