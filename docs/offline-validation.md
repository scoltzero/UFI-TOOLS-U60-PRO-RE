# Offline validation

No command in this checklist contacts a U60 Pro.

```sh
go version # must report go1.26.10 or newer within the 1.26 line
make test
make lint
go list -e ./... | rg -v '/third_party/' | xargs go test -race
go mod verify
make vuln
make build-arm64
make build-installers
make android-check # requires JDK 17 and Android SDK 35 in the environment
make installer-test
make license-test
node --check internal/webui/static/app.js
file bin/ufi-tools-u60pro-linux-arm64 bin/verify-release-linux-arm64 bin/UFI-TOOLS-U60-Pro-Installer.exe
sh -n installer/install-adb.sh installer/bundle/service.sh
sh installer/test-install-adb.sh
```

Expected build properties:

- the service, verifier, and plugin-store targets are Linux ELF AArch64;
- Android lint reports no issues and the debug APK is launchable and v2-signed;
- target binaries are statically linked;
- unit tests use temporary directories and the deterministic device simulator;
- the vulnerability scan reports zero reachable vulnerabilities;
- installer transport defaults to dry-run and requires `--execute` to invoke ADB;
- unit tests do not discover or open a network connection to a device.

The separate authorized device record is in `docs/device-validation.md`.
