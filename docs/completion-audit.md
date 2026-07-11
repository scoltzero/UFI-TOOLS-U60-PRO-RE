# Non-release completion audit

Audit scope: all requested implementation and validation work except signing,
publishing, and official release distribution.

| Requirement | Evidence | Result |
|---|---|---|
| Repository and version history | Local `UFI-TOOLS-U60-PRO-RE`, private GitHub origin, stepwise commits preserved by PR #1 merge into default branch `main` | Complete |
| Open-source provenance | `UPSTREAM.lock`, upstream integration notes, exact MIT notice, clean-room policy | Complete |
| Reference-service analysis | Sanitized service/route map records the plugin, update, device-reporting, notification, blacklist and CAPTCHA endpoints without retaining credentials or private responses | Complete |
| Static U60 runtime | Go service, SQLite migrations, embedded UI, Linux/ARM64 static build | Complete offline |
| U60 device adapter | Dedicated UBUS backend calibrated on the authorized U60 Pro; generic GoForm adapter retained | Read-only core validated on device |
| Observed route compatibility | Executable route registration test plus implemented handlers; no compatibility handler returns 501 | Complete offline |
| Authentication | Generated persistent token, constant-time comparison, optional legacy signature, guarded shell | Complete |
| Plugins/store | 41 public plugins migrated into an ignored verified snapshot; format/hash/link/risk audit plus self-hosted frontend integration | Complete; third-party source not republished without license review |
| Background behavior | Scheduler, cellular sampling, SMS polling/forwarding and cancellation tests | Complete offline |
| APN/ADB/ttyd/hosts | U60 command contracts, fixed privileged commands, process supervision, atomic hosts rollback | Complete offline |
| Updates | Ed25519 manifest, size/SHA-256, same-origin download, staging and transactional installer | Complete offline |
| ADB installation | Dry-run default, architecture preflight, signed bundle and rollback transaction test | Complete offline |
| LAN restore installation | Real backup decrypt/inspect, multipart upload, restore RPC, mandatory matching rollback container, candidate and rollback down/up health proof | Format calibrated; destructive restore not invoked |
| Graphical installers | Native Android source and single-file Windows loopback Web UI using the shared transactional transport | Windows PE32+ verified; Android SDK 35 debug APK built, v2 signature and launchable Activity verified |
| Proprietary material exclusion | Git ignore rules and repository search; APK/EXE/Qualcomm files absent | Complete |
| Licensing | Full AGPL text, upstream MIT notice, automated dependency-license collection | Complete |
| Toolchain security | Go 1.26.5 is required locally and in CI; `govulncheck@v1.6.0` reports zero reachable vulnerabilities | Complete |
| CI | Tests, vet, module verification, vulnerability scan, ARM64 build and installer rollback workflow on merged `main` | Complete; run `29138245168` passed |

## Remaining destructive gate

The ARM64 candidate was run safely from `/tmp` on port 2433 and its UBUS-backed
device summary, legacy reads, complete UI, storage, and health endpoints were
validated. The original service remained on port 2333 and all temporary files
were removed afterward.

The user-owned vendor backup was decrypted with the calibrated device-bound
password derivation. Its exact outer pair and inner MD5 passed, and all 171
inner entries were inspected, including two contained relative symlinks. Both
the open candidate and a matching closed-service rollback container were then
generated and inspected offline.

One gate remains intentionally unclaimed: accepting the candidate container,
rebooting, proving candidate health, deliberately exercising rollback, and
comparing the restored baseline. The device web interfaces are reachable, but
network ADB is offline and anonymous UBUS correctly denies restore management
operations; anonymous CGI upload returns HTTP 405. The independently tested
official router-password login and `webtoken` upload flow is implemented in all
installers. A router Web password, physically connected authorized ADB
transport, or an authorized 32-character UBUS session is required before this
destructive gate can run.
