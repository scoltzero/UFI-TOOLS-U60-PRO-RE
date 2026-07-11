# Offline validation

No command in this checklist contacts a U60 Pro.

```sh
go version # must report go1.26.5 or newer within the 1.26 line
go test ./...
go vet ./...
go test -race ./...
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
make build-arm64
make installer-test
node --check internal/webui/static/app.js
file bin/ufi-tools-u60pro-linux-arm64 bin/verify-release-linux-arm64
sh -n installer/install-adb.sh installer/bundle/service.sh
```

Expected build properties:

- the service, verifier, and plugin-store targets are Linux ELF AArch64;
- target binaries are statically linked;
- unit tests use temporary directories and the deterministic device simulator;
- the vulnerability scan reports zero reachable vulnerabilities;
- installer transport defaults to dry-run and requires `--execute` to invoke ADB;
- no test discovers or opens a network connection to a device.

Device validation is a separate, future authorization gate. Before it occurs,
capture a full backup, verify rollback manually, use a disposable test unit if
possible, and record firmware/model identifiers in a redacted compatibility
report.
