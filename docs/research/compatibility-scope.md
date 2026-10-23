# Compatibility scope

Current target (2026-10-22): main-function usability, as defined in [PLAN.md](../../PLAN.md), rather than complete reference behavior parity. This document is the sanitized handoff from research to implementation.
Decompiler output stays in the ignored `research-inputs/` directory.

## Runtime contract

- Target OS/architecture: Linux ARM64.
- Supported isolated script installation: `/data/ufi-tools-re`, port `2433`; original `/data/ufi-tools` and `2333` stay in place.
- The standalone code defaults still use the old root/port; direct development startup must pass an explicit separate data directory and address. This note changes documentation, not runtime defaults.
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

## Scope choices and remaining functional work

The self-hosted/configurable store, signed update validation and opt-in legacy
authentication/shell are intentional choices. They need not be replaced by the
reference's defaults. Original exclusive cloud registration/reporting, UUID
checks and official-store/OTA protocols are not default deliverables.

Remaining main-function gaps are tracked in PLAN.md: real homepage telemetry,
U60 device write/command integration, frontend/backend task and usage contracts,
actual background effects, selected plugin execution, and physical validation.
These are distinct from optional old-data import and exact-behavior research.

Keep contracts needed by current consumers or update both ends. Preserve
original/user data and existing authentication, integrity and recovery guarantees.
