# Replica policy

## Functional target (updated 2026-10-22)

The reference is the authorized closed `ufi-tools-u60pro` 0.3.2 build for the
U60 Pro. The current objective is a usable independent implementation of its
main functions. [PLAN.md](../PLAN.md) is the current scope and progress source;
the original question bank remains an unchanged research reference.

A selected function is complete when the actual UI or consumer can perform
its operation, values and units are correct, required settings persist, failures
are reported accurately, and original-system/user data protections remain intact.
Local checks and physical-device verification are recorded separately.

Preserve the API contracts actually used by the chosen frontend, plugins and
DevUI/datad, or update those consumers together. Do not require a field-by-field
match to every original response, identical wording/timing, the same database or
internal layout, or reproduction of original defects. Reverse engineer only a
specific implementation gap; a full reference behavior baseline is not a gate.

## Script-install scope (2026-10-21)

At the owner's request, stage 2 now delivers an isolated shell installer only.
It uses `/data/ufi-tools-re` and port 2433 so the original system and service
remain in place. The owner subsequently restored LAN script delivery (first
install/uninstall/inverse recovery); it reboots through the vendor restore
mechanism and still needs hardware validation. Windows/Android GUI delivery
remains cancelled. This is
the supported independent installation layout. Main-function completion
and physical validation are tracked in PLAN.md. Initial script startup
keeps automatic write workers, AT and fixed privileged operations disabled.

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

## Independent implementation choices

Self-hosted/configurable plugin sources, signed updates and opt-in legacy shell
are intentional choices, not automatic parity defects. The reference's exclusive
cloud registration/reporting, UUID checks, official-store and official-OTA
protocols are not default requirements for this independent main-function target.
Do not alter the original service or bypass its controls. Existing authentication,
validation, update integrity and data-recovery protections remain in place.

Reference timing, empty values or error wording need further study only when a
supported consumer or functional failure makes them relevant. Existing evidence
is retained with its original limits; a cancelled or untestable scenario is not
reported as passed.
