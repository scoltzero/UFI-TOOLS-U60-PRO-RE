# UFI-TOOLS-U60-PRO-RE

A behavior-level replica of the closed `ufi-tools-u60pro` 0.3.2 service for
the ZTE U60 Pro (MU5250), built from authorized reverse-engineering research.
The web interface is a U60 Pro fork of the MIT-licensed upstream
[kanoqwq/UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS) frontend.

> Status: v0.0.1 development line. Route surface, storage, plugins and
> installers exist; the ubus-backed GoForm emulation, OTA, factory-reset
> tracking, device registration/reporting and the install/uninstall lifecycle
> are being brought to parity with the reference. See
> [Replica policy](docs/replica-policy.md) for the parity target.

## Goals

- Run as a static Linux/ARM64 service with an embedded web interface, using the
  reference layout (`/data/ufi-tools`, port 2333, `service.sh`, `rc.local`).
- Reproduce the reference API field by field: names, types, units, defaults,
  error semantics and side effects, verified against captured responses.
- Follow the reference's choices, including its official plugin store, device
  registration/reporting, notifications and blacklist check.
- Install, upgrade, roll back and uninstall cleanly on the U60 Pro.

## Non-goals

- Redistributing the closed U60 Pro build or its decompiled output.
- Redistributing Qualcomm tools, drivers, loaders, or firmware.
- Commercial use or public redistribution of the research inputs.

## Repository policy

Implementation may be guided by decompiled evidence of the reference build
(Ghidra/REA output, decrypted strings, route tables) and by captured traffic.
That evidence stays in the Git-ignored `research-inputs/` directory; only
independently written source, tests and sanitized fixtures are committed.

## Local development

```sh
make test
go run ./cmd/ufi-tools-u60pro --data-dir "$PWD/tmp/data" --http-addr 127.0.0.1:2333
```

Open `http://127.0.0.1:2333`. The development service uses a deterministic U60
Pro simulator and never falls through to a connected device. On first start it
generates a token in `tmp/data/device-token`; enter that token in the web UI.

Build the target binaries with `make build-arm64` (the version defaults to
`0.0.1`; override with `make VERSION=x.y.z`). Installation is documented under
`installer/`; its ADB transport is dry-run-only unless `--execute` is supplied.

## License

New code is licensed under `AGPL-3.0-or-later`. Reused upstream UFI-TOOLS code
retains its MIT license. See `LICENSE`, `LICENSES/`, and `NOTICE`.
