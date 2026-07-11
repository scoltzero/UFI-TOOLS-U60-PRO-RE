# LAN backup/restore transport

This directory reserves the second installation transport for devices where an
authorized ADB bootstrap is unavailable. It is intentionally not executable
yet: emitting a guessed vendor backup container would be unsafe.

Implementation acceptance criteria:

- derive the container format from user-owned, redacted before/after fixtures;
- parse and regenerate it deterministically without proprietary Qualcomm tools;
- validate all path, length, checksum, and model/version fields;
- produce a preview manifest showing every modified path;
- require an explicit `--execute` confirmation for network restore;
- preserve the original backup and implement an automatic rollback artifact.

The payload and transaction must remain identical to the ADB bundle. Only the
transport/container adapter differs.

