# Non-release completion audit

Audit scope: all requested offline implementation and validation work except
signing, publishing, official release distribution, and—at the owner's explicit
request—device mutation, reboot, destructive restore, and real-device fault
injection.

| Requirement | Evidence | Result |
|---|---|---|
| Repository and version history | Local `UFI-TOOLS-U60-PRO-RE`, private GitHub origin, stepwise commits preserved by PR #1 merge into default branch `main` | Complete |
| Open-source provenance | `UPSTREAM.lock`, upstream integration notes, exact MIT notice, clean-room policy | Complete |
| Reference-service analysis | Sanitized service/route map records plugin, update, reporting, notification, blacklist, CAPTCHA, author and legacy-UI endpoints without retaining credentials or private responses | Complete |
| Static U60 runtime | Go service, SQLite migrations, embedded UI, Linux/ARM64 static build | Complete offline |
| U60 device adapter | Dedicated UBUS backend calibrated on the authorized U60 Pro; generic GoForm adapter retained | Read-only core validated on device |
| Observed route compatibility | Executable route registration test plus implemented handlers; no compatibility handler returns 501 | Complete offline |
| Authentication | Generated persistent token, constant-time comparison, optional legacy signature, guarded shell | Complete |
| Plugins/store | 41 public plugins migrated into an ignored verified snapshot; format/hash/link/risk audit plus self-hosted frontend integration | Complete; third-party source not republished without license review |
| Background behavior | Scheduler, cellular sampling, SMS polling/forwarding and cancellation tests | Complete offline |
| APN/ADB/ttyd/hosts | U60 command contracts, fixed privileged commands, process supervision, atomic hosts rollback | Complete offline |
| Updates | Ed25519 manifest, size/SHA-256, same-origin download, staging and transactional installer | Complete offline |
| ADB installation | Dry-run default, architecture preflight, signed bundle and rollback transaction test | Complete offline |
| LAN restore installation | Real backup decrypt/inspect, multipart upload, restore RPC, mandatory matching rollback container, password-based re-authentication after reboot, and candidate/rollback down-up health proof | Complete offline; destructive restore intentionally not invoked |
| Graphical installers | Native Android source and single-file Windows loopback Web UI using the shared transactional transport | Windows PE32+ verified; Android SDK 35 debug APK built, v2 signature and launchable Activity verified |
| Proprietary material exclusion | Git ignore rules and repository search; APK/EXE/Qualcomm files absent | Complete |
| Licensing | Full AGPL text, root and embedded-frontend MIT attribution, automated dependency-license collection isolated from ignored research trees | Complete |
| Toolchain security | Go 1.26.5 is required locally and in CI; `govulncheck@v1.6.0` reports zero reachable vulnerabilities | Complete |
| Validation automation | Tests, vet, race detector, module verification, vulnerability scan, ARM64/Windows/Android builds, installer rollback, ADB diagnostics and license collection | Complete locally; historical merged-main CI run `29138245168` passed, current non-release branch was not published |

## Final offline validation evidence

The final non-release pass completed the following without contacting the U60
Pro:

- all Go tests, Vet, race-detector tests, and module verification passed;
- `govulncheck@v1.6.0` reported zero reachable vulnerabilities;
- the service, verifier, plugin store, and AT helper rebuilt as stripped,
  statically linked Linux AArch64 ELF executables;
- the CLI and graphical Windows installers rebuilt as PE32+ x86-64 executables;
- Android lint reported no issues and the SDK 35 debug APK rebuilt with one
  launchable activity, only the Internet permission, and APK Signature Scheme
  v2 verification (debug signing only, not a release signature);
- simulated transactional install, failed-candidate rollback, post-reboot
  password re-authentication, and ADB state diagnostics passed;
- the dependency-license collector produced ten dependency entries while an
  ignored upstream research tree was present, proving the scope exclusion;
- the 41-plugin snapshot re-audited as 25 comment-script, 11 script, and 5
  marker-format plugins, with no plugin code executed;
- the original authorized backup re-inspected as 171 contained paths with two
  safe relative symlinks; the regenerated candidate and rollback containers
  re-inspected as eight and three contained paths respectively.

## Deferred real-device validation

Before the current no-device-mutation scope was established, the ARM64
candidate had been run non-persistently from `/tmp` on port 2433 and its
UBUS-backed device summary, legacy reads, complete UI, storage, and health
endpoints were validated. The original service remained on port 2333 and all
temporary files were removed afterward.

The user-owned vendor backup was decrypted with the calibrated device-bound
password derivation. Its exact outer pair and inner MD5 passed, and all 171
inner entries were inspected, including two contained relative symlinks. Both
the open candidate and a matching closed-service rollback container were then
generated and inspected offline.

Real-device candidate restore, reboot, candidate health validation, fault
injection, rollback, and restored-baseline comparison are intentionally outside
the current scope. They are not prerequisites for this offline deliverable and
must not be run without a new, operation-specific authorization from the device
owner.

The rollback container restores the captured core service, service script, and
startup entry; it is not a full partition image and does not guarantee a
byte-for-byte return to the prior system. Any future destructive validation
should use a spare device or first establish and independently verify a
partition-level EDL/Firehose recovery path.

No candidate container was uploaded or restored during this completion pass.
