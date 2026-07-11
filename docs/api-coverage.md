# API coverage

Coverage is tracked by behavior rather than private implementation details.

| Family | Current state | Notes |
|---|---|---|
| Health/version | Implemented | Public readiness and build identity |
| Token discovery | Implemented | `/api/need_token` |
| Primary token auth | Implemented | Constant-time header/Bearer comparison |
| Legacy timestamp signature | Compatibility profile | Opt-in and clock-skew limited |
| Configuration | Partial | Nickname persistence and security/store flags |
| Device status | Simulated | Typed device interface; no host/device fallback |
| ZTE GoForm | Simulated | Get/set contract with deterministic simulator |
| Time | Partial | Status implemented; mutation blocked in simulator |
| Remote shell | Guarded stub | Disabled by default; no command runner configured |
| Plugin custom head | Implemented | Four historical formats and size validation |
| Plugin catalog | Local skeleton | Empty self-hosted catalog; no default remote |
| Hosts | Planned | Persistence and apply/restore adapter |
| Scheduler | Planned | SQLite schema present |
| SMS forwarding | Planned | SQLite schema present |
| Cellular history | Planned | SQLite schema present |
| Uploads | Planned | SQLite schema present; request limit established |
| APN/cellular controls | Planned | Requires sanitized ZTE behavior fixtures |
| ADB/ttyd lifecycle | Planned | Must be behind explicit process policy |
| OTA | Verification implemented | Ed25519 manifest plus size and SHA-256 artifact check |

“Simulated” means route behavior can be developed and tested without accessing
a U60 Pro. It does not claim that the device adapter has been validated.

