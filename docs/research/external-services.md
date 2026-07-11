# External service map

Observed facts are separated from project decisions. No credential, unique
device ID, or private response body is retained here.

## Reference service

- Web/plugin service: `https://ufitools.ikuns.top/`.
- Public plugin listing: `/api/v1/plugins`.
- Public plugin download: `/api/v1/plugins/{public_name}/download`.
- Update API strings observed in installers/core: `/api/v1/updates` and related
  changelog/download routes.
- Device control-plane strings observed in the core: `/api/v1/devices/register`,
  `/api/v1/devices/report`, notification polling, and a UUID blacklist query.
- The storefront Content-Security-Policy observed on 2026-07-11 allowed
  connections to `https://cap.fn0721.top` for CAPTCHA functionality.
- The storefront advertised plugin/update distribution and returned a public
  catalog with 41 entries at the time of observation.

## Behavior and risk assessment

The reference core periodically registered/reported the device and polled
notifications/updates. A delayed then recurring blacklist check could cause the
service to edit its startup entry and stop itself. These behaviors are not
required for U60 Pro interoperability and are intentionally absent here.

Reference OTA behavior checked size and ELF identity but did not establish an
independent offline trust root. This project instead requires an Ed25519-signed
manifest plus exact artifact size and SHA-256.

## Open replacement

`cmd/ufi-plugin-store` serves a minimal self-hosted catalog with immutable
digests. `cmd/import-legacy-store` can explicitly snapshot a compatible legacy
HTTPS store on the administrator's computer, validate each plugin, calculate
digests, and produce a local catalog. The running U60 service never defaults to
the reference domain.

