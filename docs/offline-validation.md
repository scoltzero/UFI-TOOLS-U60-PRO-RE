# Offline validation

No command in this checklist contacts a U60 Pro.

```sh
go test ./...
go vet ./...
make build-arm64
file bin/ufi-tools-u60pro-linux-arm64 bin/verify-release-linux-arm64
sh -n installer/install-adb.sh installer/bundle/service.sh
```

Expected build properties:

- both target binaries are Linux ELF AArch64 executables;
- both are statically linked;
- unit tests use temporary directories and the deterministic device simulator;
- installer transport defaults to dry-run and requires `--execute` to invoke ADB;
- no test discovers or opens a network connection to a device.

Device validation is a separate, future authorization gate. Before it occurs,
capture a full backup, verify rollback manually, use a disposable test unit if
possible, and record firmware/model identifiers in a redacted compatibility
report.

