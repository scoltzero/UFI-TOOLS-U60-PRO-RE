# Clean-room implementation policy

## Allowed inputs

- The MIT-licensed upstream UFI-TOOLS source, with attribution retained.
- Public documentation and public network interfaces.
- Black-box observations made on equipment the researcher is authorized to use.
- Sanitized specifications describing requests, responses, state transitions,
  file locations, timing, and failure behavior.

## Prohibited repository content

- Closed executable code or reconstructed/decompiled source.
- Copied private strings or assets except the minimum needed to identify an
  interoperable protocol.
- Credentials, signing keys, device tokens, unique identifiers, or user data.
- Proprietary Qualcomm tools, drivers, programmers, or firmware images.

## Implementation rule

Code authors implement behavior from a written compatibility specification and
tests. Research notes distinguish observed fact, inference, and new design.
Every imported open-source component carries its original license notice in the
file or an adjacent directory-level attribution file. The Git history should
make provenance review possible.

## Security divergence

Compatibility does not require reproducing harmful control-plane behavior.
The open implementation must not include an external blacklist, remote stop
switch, undisclosed telemetry, or unsigned self-update. Any optional remote
service is explicit, documented, and disableable.
