# Android graphical installer

This is a clean-room native Android implementation of the documented U60 Pro
LAN backup/restore protocol. It does not contain files, resources, or code from
the supplied closed APK.

The app selects a locally generated `backup_restore.bin`, validates the private
device origin and optional UBUS session, and defaults to preview-only mode.
Execution requires both the checkbox and the exact phrase `RESTORE U60 PRO`.
It saves the official backup in app-private storage before upload, requires a
down/up health transition, and triggers the original backup restore on failure.

Build with JDK 17 and Android SDK 35:

```sh
./gradlew :app:assembleDebug
```

The debug APK is written under `app/build/outputs/apk/debug/`. Signing and
publishing are deliberately outside this repository workflow.
