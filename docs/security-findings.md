# Security notes on the reference

Analysis of the reference deployment identified these architectural risks.
The replica follows the reference (see `docs/replica-policy.md`); the list is
kept so that users know what the reference behavior implies:

- periodic third-party registration/reporting and notification/update polling;
- an external blacklist decision capable of stopping or disabling the service;
- update acceptance without an independently trusted artifact signature;
- weak legacy authentication and a high-impact shell endpoint;
- persistent browser-executed plugins without a first-party trust model.

The current replica still lacks the control-plane client and still requires
signed updates; both are pending parity work. Exact tokens, unique device
identifiers, and copied private messages are not stored in this repository.

