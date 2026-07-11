# API coverage

Coverage is tracked by behavior rather than private implementation details.

| Family | Current state | Notes |
|---|---|---|
| Health/version | Implemented | Public readiness and build identity |
| Token discovery | Implemented | `/api/need_token` |
| Primary token auth | Implemented | Constant-time header/Bearer comparison |
| Legacy timestamp signature | Compatibility profile | Opt-in and clock-skew limited |
| Configuration | Implemented | Persistent identity, security, service and store flags |
| Device status | Adapter complete | Simulator and explicit private-address ZTE HTTP backend |
| ZTE GoForm | Implemented | Single/multi get and command-result contracts |
| Time | Implemented | Status plus guarded device-only mutation |
| Remote shell | Implemented, opt-in | Authentication, timeout, output cap and exit status |
| Plugin custom head | Implemented | Four historical formats and size validation |
| Plugin catalog | Implemented | Self-hosted verified catalog, legacy snapshot importer and lifecycle aliases |
| Hosts | Implemented | One-time backup, atomic apply and restore; disabled in simulator |
| Scheduler | Implemented | CRUD plus cancellable one-shot/periodic GoForm worker |
| SMS forwarding | Implemented | ZTE polling, dedupe, blacklist, CURL, DingTalk, TLS SMTP and SMS sinks |
| Cellular history | Implemented | Bounded recent-point persistence/query |
| Uploads | Implemented | SQLite blob storage, safe names, 1 MiB limit |
| APN/cellular controls | Implemented | U60 `APN_PROC_EX` profile list/save/delete/select and traffic worker |
| ADB/ttyd lifecycle | Implemented | Fixed ADB commands and PID/log-supervised ttyd process |
| OTA | Implemented | Same-origin fetch, Ed25519, size/SHA-256, staging and transactional install |
| LAN restore installer | Offline implementation | Archive/MD5/OpenSSL-3DES generation; real restore intentionally untested |

“Simulated” means route behavior can be developed and tested without accessing
a U60 Pro. It does not claim that the device adapter or vendor restore parser
has been validated on physical hardware.
