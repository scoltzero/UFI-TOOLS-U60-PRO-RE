# UFI-TOOLS-U60-PRO-RE

An open, auditable UFI-TOOLS-compatible management service for the ZTE U60 Pro.
The project combines reusable MIT-licensed concepts from the upstream
[kanoqwq/UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS) project with a
clean-room reimplementation of device-specific observable behavior.

> Status: implementation-complete candidate with real-device read-only and
> backup-format calibration. Destructive installation/reboot/rollback
> validation remains an explicit authorized-device gate.

## Goals

- Run as a static Linux/ARM64 service with an embedded web interface.
- Preserve useful legacy API and plugin compatibility where practical.
- Support a self-hosted plugin store and independently signed updates.
- Provide reproducible ADB and LAN-backup installers with safe rollback.
- Remove opaque telemetry, remote blacklist/kill-switch behavior, and reliance
  on a third-party control plane.

## Non-goals

- Redistributing the closed U60 Pro build or its decompiled source.
- Redistributing Qualcomm tools, drivers, loaders, or firmware.
- Bypassing device ownership, access controls, or carrier restrictions.
- Claiming source-level equivalence with the closed implementation.

## Repository policy

Only independently written source, public protocol facts, tests, and properly
attributed open-source code belong here. Research inputs stay outside Git and
are represented by sanitized behavior specifications and test fixtures.

See [Architecture](docs/architecture.md), [Clean-room policy](docs/clean-room.md),
and [Threat model](docs/threat-model.md). Current endpoint coverage is tracked
in [API coverage](docs/api-coverage.md).

## Local development

```sh
go test ./...
go run ./cmd/ufi-tools-u60pro --data-dir "$PWD/tmp/data" --http-addr 127.0.0.1:2333
```

Open `http://127.0.0.1:2333`. The development service uses a deterministic U60
Pro simulator and never falls through to a connected device. On first start it
generates a token in `tmp/data/device-token`; enter that token in the web UI.

Build the target binaries with `make build-arm64`. Installation is documented
under `installer/`; its ADB transport is dry-run-only unless `--execute` is
explicitly supplied.

The web dashboard exposes device status, ADB/ttyd controls, hosts management,
verified plugins, scheduling and SMS-forwarding configuration. The backend also
implements the observed legacy API surface for upstream frontend/plugin
compatibility.

## License

New code is licensed under `AGPL-3.0-or-later`. Reused upstream UFI-TOOLS code
retains its MIT license. See `LICENSE`, `LICENSES/`, and `NOTICE`.
