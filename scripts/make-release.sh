#!/bin/sh
set -eu

VERSION=${VERSION:?set VERSION, for example 0.1.0}
PRIVATE_KEY=${PRIVATE_KEY:?set PRIVATE_KEY to an Ed25519 private key file}
PUBLIC_KEY=${PUBLIC_KEY:?set PUBLIC_KEY to the matching public key file}
OUT=${OUT:-build/release-$VERSION}
ARCHIVE=$PWD/build/UFI-TOOLS-U60-PRO-RE-$VERSION-linux-arm64.tar.gz

make build-arm64
rm -rf "$OUT"
mkdir -p "$OUT"
cp bin/ufi-tools-u60pro-linux-arm64 "$OUT/ufi-tools-u60pro"
cp bin/verify-release-linux-arm64 "$OUT/verify-release"
cp installer/bundle/service.sh "$OUT/service.sh"
cp bin/sendat-linux-arm64 "$OUT/sendat"
cp bin/ufi-plugin-store-linux-arm64 "$OUT/ufi-plugin-store"
cp -R plugin-store "$OUT/plugin-store"
cp "$PUBLIC_KEY" "$OUT/public-key.hex"
cp LICENSE NOTICE "$OUT/"
./scripts/collect-licenses.sh "$OUT/third-party-licenses"
if [ -n "${TTYD_BINARY:-}" ]; then cp "$TTYD_BINARY" "$OUT/ttyd"; chmod 0755 "$OUT/ttyd"; fi
if [ -n "${SPEEDTEST_BINARY:-}" ]; then cp "$SPEEDTEST_BINARY" "$OUT/speedtest"; chmod 0755 "$OUT/speedtest"; fi
chmod 0755 "$OUT/ufi-tools-u60pro" "$OUT/verify-release" "$OUT/service.sh" "$OUT/sendat" "$OUT/ufi-plugin-store"

go run ./cmd/sign-release \
  -version "$VERSION" \
  -artifact "$OUT/ufi-tools-u60pro" \
  -private-key "$PRIVATE_KEY" \
  -output "$OUT/manifest.json"

go run ./cmd/verify-release \
  -manifest "$OUT/manifest.json" \
  -artifact "$OUT/ufi-tools-u60pro" \
  -public-key "$OUT/public-key.hex"

mkdir -p build
tar -C "$OUT" -czf "$ARCHIVE" .
echo "created $ARCHIVE"
