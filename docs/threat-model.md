# Threat model

The daemon manages a network device and may run with elevated privileges, so a
web/API compromise can become a device compromise.

## Protected assets

- Device configuration, messages, traffic records, and credentials.
- Service integrity and availability.
- Update signing keys and plugin trust decisions.
- Other clients reachable from the device network.

## Principal threats

- Unauthenticated LAN access or replay of legacy request signatures.
- Arbitrary command execution through compatibility shell endpoints.
- Malicious plugins gaining persistent script execution in the management UI.
- Supply-chain replacement of the daemon, UI, installer, or plugin catalog.
- Third-party telemetry, blacklist, or kill-switch endpoints.
- Partial installation leaving the device unable to start or roll back.

## Required controls

- Bind and authentication defaults that fail closed.
- Constant-time token comparison, request size/time limits, and audit records.
- Shell compatibility disabled unless explicitly enabled; commands constrained
  by policy when it is enabled.
- Plugin provenance and explicit trust state; no silent remote execution.
- Offline public-key verification for release manifests and artifacts.
- Transactional install with preflight checks, backups, health checks, and
  rollback.
- No mandatory external service and no hidden outbound requests.

