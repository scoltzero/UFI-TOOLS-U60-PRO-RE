# Replica policy

## Parity target

The reference is the closed `ufi-tools-u60pro` 0.3.2 build for the U60 Pro
(go1.25.0, VCS revision `fe229709c6e4`, built 2026-08-15). The replica matches
its observable behavior: routes, request parsing, response fields and types,
error messages, persisted state, background workers, outbound services, file
layout, and install/upgrade/uninstall lifecycle.

Behavior is "done" for a route or GoForm key only when its response has been
compared field by field with a captured reference response from the same
device state, and any side effect has been checked on the device and reverted.

## Allowed inputs

The owner of the reference authorized reverse engineering for study and
non-commercial reimplementation. Allowed inputs are:

- the MIT-licensed upstream UFI-TOOLS source, with attribution retained;
- static analysis of the reference build: Ghidra/REA decompilation, decrypted
  string literals, function and route tables;
- traffic and ubus/UCI observations captured on the owner's own device;
- black-box comparison of reference and replica responses.

## Repository content

- Decompiled output, the reference binary and raw captures stay in the
  Git-ignored `research-inputs/` directory and are never committed.
- Committed code is written for this project. Tests use sanitized fixtures
  that contain no credentials, tokens, IMEI/ICCID/MAC values or messages.
- Proprietary Qualcomm tools, drivers, programmers and firmware images are
  never committed.

## Reference behaviors kept on purpose

The replica follows the reference rather than redesigning it. This includes
the official plugin store, device registration/reporting, notifications,
update checks, and the UUID blacklist check. Where the replica still differs
(for example signed updates or opt-in legacy shell), the difference is listed
in `docs/research/compatibility-scope.md` as pending parity work, not as a
policy.
