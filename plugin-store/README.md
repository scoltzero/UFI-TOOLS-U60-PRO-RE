# Self-hosted plugin store

Run locally with:

```sh
go run ./cmd/ufi-plugin-store --root ./plugin-store --http-addr 127.0.0.1:2340
```

`catalog.json` is versioned, each plugin has a stable ID/version, and the
service computes and verifies size/SHA-256 before serving content. Fill the
digest and size fields for immutable production catalogs; zero/empty values are
accepted only as development-time auto-compute values.

Set `UFI_STORE_TOKEN` to require the `X-Store-Token` header. Deploy behind HTTPS
outside a trusted LAN. The catalog does not provide telemetry, registration,
blacklisting, remote stop, or executable core updates.

## Snapshot an existing compatible store

An administrator may explicitly migrate a legacy HTTPS store into an immutable
self-hosted snapshot:

```sh
go run ./cmd/import-legacy-store \
  --url https://legacy-store.example \
  --output ./private-plugin-snapshot
```

Redirects to external file hosts are denied by default. If the listing's
download endpoint intentionally redirects, inspect that destination and allow
its exact HTTPS hostname explicitly, for example:

```sh
go run ./cmd/import-legacy-store \
  --url https://ufitools.ikuns.top \
  --allow-download-host tapan.top \
  --output ./private-plugin-snapshot
```

The importer validates every supported plugin document, enforces the size
limit, gives files stable IDs, computes SHA-256 digests, and writes a new
`catalog.json`. The core service should point to the resulting self-hosted
catalog instead of directly trusting unsigned legacy downloads.

Audit the immutable snapshot without executing any plugin code:

```sh
go run ./cmd/audit-plugin-store \
  --root ./private-plugin-snapshot \
  --output ./plugin-audit.json
```

The report records each detected format, HTTP(S) URL, hostname, local API path,
and high-risk construction such as shell APIs, dynamic evaluation, recursive
deletion, or command-line downloads. Findings are review prompts rather than a
claim that a plugin is malicious or safe.
