# Compatibility scope

This document is the sanitized handoff from behavior research to implementation.
It intentionally contains no decompiler output.

## Runtime contract

- Target OS/architecture: Linux ARM64.
- Default root: `/data/ufi-tools`.
- Default HTTP address: `:2333`.
- Optional terminal helper port: `1146`.
- State: SQLite tables for config, key/value data, schedulers, SMS forwarding,
  recent/daily cellular totals, and uploaded blobs.

## API families

- Authentication and configuration.
- Device status and ZTE `goform` compatibility.
- Time, hosts, scheduler, SMS forwarding, and traffic accounting.
- APN, cellular controls, ADB and terminal lifecycle.
- Plugin storage/rendering and plugin-store proxying.
- Uploads and update status.

The route matrix, field schemas, and fixtures will be maintained as code-backed
contracts under `internal/httpapi` and `testdata` rather than copied handler
implementations.

## Plugin formats to recognize

- A normal HTML `<script>` fragment.
- The historical `//<script>` prefix.
- `KANO_PLUGIN_START`, metadata, and `KANO_PLUGIN_END` markers.
- The historical `kano_disabled` wrapper.

## Intentional differences

- The default store is self-hosted/configurable.
- Update artifacts require a trusted signature.
- Telemetry and external blacklist checks are absent.
- Legacy authentication and shell behavior are opt-in compatibility modes.

