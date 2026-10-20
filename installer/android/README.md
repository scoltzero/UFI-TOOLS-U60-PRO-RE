# Android graphical installer

This is an independently written native Android implementation of the documented U60 Pro
LAN backup/restore protocol. It does not contain files, resources, or code from
the supplied closed APK.

The app selects a locally generated `backup_restore.bin`, validates the private
device origin and optional UBUS session, and defaults to preview-only mode.
Execution requires both the checkbox, the exact phrase `RESTORE U60 PRO`, and
a verified rollback container matching the device's currently installed
service and `rc.local`. It saves a private safety copy before upload, requires
a down/up health transition, and verifies the same transition after rollback.
Execution also requires either the router Web password or an existing UBUS
session. Password login uses the official salted two-pass SHA-256 flow, keeps
the password only in memory, and sends the resulting `webtoken` cookie for the
CGI upload.

Build with JDK 17 and Android SDK 35:

```sh
./gradlew :app:assembleDebug
```

The debug APK is written under `app/build/outputs/apk/debug/`. Signing and
publishing are deliberately outside this repository workflow.
