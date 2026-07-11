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

