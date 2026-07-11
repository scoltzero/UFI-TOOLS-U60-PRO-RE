# Security-driven compatibility decisions

Black-box analysis of the reference deployment identified architectural risks
that are not reproduced in this implementation:

- periodic third-party registration/reporting and notification/update polling;
- an external blacklist decision capable of stopping or disabling the service;
- update acceptance without an independently trusted artifact signature;
- weak legacy authentication and a high-impact shell endpoint;
- persistent browser-executed plugins without a first-party trust model.

Accordingly, this project has no default outbound control plane, omits remote
blacklist/stop behavior, requires Ed25519 release signatures, disables legacy
authentication and shell execution by default, and records plugin format/trust
state. Exact endpoint hostnames, tokens, unique device identifiers, and copied
private messages are not needed for implementation and are not stored here.

