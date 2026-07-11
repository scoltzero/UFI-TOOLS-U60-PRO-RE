# Architecture

## Target layout

The target runtime is one static `linux/arm64` Go daemon installed under
`/data/ufi-tools`. It serves an embedded web UI on port `2333`, stores mutable
state in SQLite, exposes a compatibility API, and supervises optional local
helpers such as `ttyd` on port `1146`.

```text
browser / legacy plugin
          |
          v
  HTTP compatibility layer
   |        |          |
 auth    core API   plugin API
   |        |          |
   +--------+----------+
            |
     typed service layer
   /      |       |       \
SQLite  ZTE API  process  update verifier
                    |
             constrained command runner
```

## Components

1. `cmd/ufi-tools-u60pro`: service entry point and lifecycle.
2. `internal/config`: defaults, environment/file overrides, validation.
3. `internal/store`: SQLite migrations and typed persistence.
4. `internal/httpapi`: routes, compatibility envelopes, and auth middleware.
5. `internal/device`: a narrow interface over U60 Pro/ZTE operations.
6. `internal/plugins`: legacy plugin parsing, storage, and rendering.
7. `internal/update`: signed manifest and artifact verification.
8. `web`: embedded first-party UI.
9. `installer`: ADB-first and LAN-backup installation transports.

Device access must be behind interfaces so the complete server can be tested
against a deterministic simulator. No test may silently fall through to ADB,
the LAN, or host shell execution.

## Compatibility strategy

Compatibility is defined by public and observable inputs/outputs: route,
method, fields, status codes, persistence effects, and plugin rendering. It is
not defined by matching private function names, control flow, or binary layout.

Legacy authentication and shell endpoints can be enabled for compatibility,
but fresh installs use safer defaults and prominently report insecure modes.

## Installation strategy

The preferred installer stages a signed release through an already-authorized
ADB connection, verifies its digest/signature, performs an atomic directory
swap, installs a service entry, runs a health check, and rolls back on failure.
A LAN backup/restore transport may be supplied for parity on devices where ADB
bootstrap is unavailable. Both transports consume the same declarative bundle
and transaction engine.

