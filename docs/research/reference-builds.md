# Reference build inventory

This is a clean-room evidence index. Hashes identify researcher-owned inputs;
the binaries themselves are excluded from Git.

## Current U60 Pro core

- Observed version: `0.2.8`.
- Packed SHA-256: `04321fe…` (abbreviated in the public document).
- Platform: statically linked Go Linux/ARM64 executable.
- Go toolchain marker: Go 1.25.
- Build module: `github.com/local/ufi-tools-u60pro/cmd/ufi-tools-u60pro`.
- Build revision marker: `5532bcfe5c594712d8ec04c6651309d9273cda09`.
- Build time marker: `2026-07-04T13:14:25Z`; modified flag set.

The module/path and private function names are evidence for component mapping,
not source to copy. Implementation in this repository is independently written.

## Installer payload

The supplied APK and Windows installer contain the same earlier core payload:

- Observed version: `0.1.4`.
- Packed SHA-256: `f38647ca…` (abbreviated).
- The Windows installer embeds ADB support and checks a downloaded installer
  SHA-256 during its own update flow.

The Qualcomm tools/driver directory is treated as proprietary third-party
material and is neither imported nor redistributed.

## Observed evolution

Compared with the installer-era payload, the current core adds host-file
management, log cleaning, SMS-forwarding state, time status/sync/configuration,
file upload, and service-worker handling. The compatibility contract tests in
`internal/httpapi` cover the currently identified public route surface.

