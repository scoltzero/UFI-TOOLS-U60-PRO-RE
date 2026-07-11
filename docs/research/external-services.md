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

## Legacy UI and author-link inventory

These strings came from the bundled public/upstream-style frontend rather than
from a claim that every address is used by the current closed core:

| Address or identifier | Observed role | Open replacement decision |
|---|---|---|
| `https://api.kanokano.cn/ufi_tools_report` | Automatic post-login message lookup keyed by device ID | Removed from the embedded UI; regression-tested as forbidden |
| `https://pan.kanokano.cn` | Legacy resource-server placeholder | Removed; resource server is blank/local by default and administrator-configurable |
| `https://cg.163.com/#/mobile` | Hidden third-party iframe/easter egg | Removed and replaced with local text |
| `https://i.kano.ink` | Disabled/commented iframe experiment | Removed from the service worker source |
| `https://t.alcy.cc/ycy` | Disabled/commented remote background | Removed from the stylesheet source |
| `https://t.me/ufi_tools_chat` | Author community link | Retained only as an explicit user-clicked link |
| `https://github.com/kanoqwq/UFI-TOOLS` | Upstream source/author attribution | Retained as attribution and documentation |
| `plugin@kano.ink` | Plugin/contact identifier in the legacy UI | Recorded as provenance; it is not contacted automatically |

The embedded runtime terms now override the legacy MIT/privacy text with the
current AGPL project policy: no author telemetry by default, and outbound data
only when an administrator explicitly configures and uses an external store,
update source, SMS webhook, or proxy-backed feature. The upstream MIT notice is
preserved separately in `NOTICE`.

## Public plugin supply-chain inventory

The explicit store snapshot contained 41 plugins: 25 historical
`//<script>` documents, 11 normal script documents, and 5 marker-based
documents. The parser also supports the disabled wrapper even though the
observed snapshot contained none. Static audit found 23 external-download
construction warnings and 19 recursive-deletion construction warnings across
the snapshot. These are review prompts, not malware verdicts; the third-party
plugin sources remain ignored and are not redistributed without license review.

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
