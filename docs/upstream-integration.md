# Upstream UFI-TOOLS integration

The MIT-licensed upstream repository is pinned by `UPSTREAM.lock` to preserve
provenance and make comparisons auditable. The selected revision is
`95cbc2cfc600863933b82a3be29edcea80504254` from the `http-server-version`
branch.

The upstream project is primarily an Android/Kotlin service plus a browser
frontend. This repository does not copy its Android binaries, bundled helper
executables, certificates, or donation/community assets into release bundles.
Reusable protocol/UI behavior is integrated selectively:

- API names and frontend request expectations inform the compatibility tests.
- GoForm request behavior is implemented independently behind `device.Device`.
- Plugin marker formats and custom-head behavior are supported by the Go core.
- The new embedded UI is independently written; a future upstream-frontend
  adapter can be tested against the compatibility router without changing the
  device backend.

Initialize the pinned source for auditing with:

```sh
./scripts/fetch-upstream.sh
```

Any future copied or modified upstream source file must retain the upstream MIT
notice. The fetched audit tree is Git-ignored and is not packaged by
`scripts/make-release.sh`.
