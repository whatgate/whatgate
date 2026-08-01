#!/usr/bin/env bash
# build-release.sh — cross-compile WhatGate binaries for all supported platforms
# and package them into per-platform archives under dist/.
#
# Produces three binaries per platform:
#   coordinator  — the coordination server
#   whatgate     — the participant (lean; SOCKS5 proxy, no TUN)
#   whatgate-tun — the participant built with -tags tun (whole-system VPN mode;
#                  needs admin/root at runtime, and wintun.dll on Windows)
#
# Usage:
#   scripts/build-release.sh [version]
# version defaults to `git describe` (e.g. v0.1.0) or "dev".
#
# Requires: go, tar, zip, sha256sum (all present on GitHub's ubuntu runners).
set -euo pipefail

# Repo root = parent of this script's dir, so it works from anywhere.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
DIST="$ROOT/dist"
LDFLAGS="-s -w -X main.version=${VERSION}"

# os/arch pairs to build. Override with WHATGATE_TARGETS="linux/amd64 ..." to
# build a subset (e.g. for a quick local check).
TARGETS="${WHATGATE_TARGETS:-
windows/amd64
windows/arm64
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
}"

echo "Building WhatGate ${VERSION}"
rm -rf "$DIST"
mkdir -p "$DIST"

for target in $TARGETS; do
	GOOS="${target%/*}"
	GOARCH="${target#*/}"
	ext=""
	[ "$GOOS" = "windows" ] && ext=".exe"

	stage="$DIST/whatgate_${VERSION}_${GOOS}_${GOARCH}"
	mkdir -p "$stage"

	echo "  -> $GOOS/$GOARCH"
	CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
		go build -trimpath -ldflags "$LDFLAGS" \
		-o "$stage/coordinator${ext}" ./cmd/coordinator
	CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
		go build -trimpath -ldflags "$LDFLAGS" \
		-o "$stage/whatgate${ext}" ./cmd/whatgate
	CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
		go build -trimpath -tags tun -ldflags "$LDFLAGS" \
		-o "$stage/whatgate-tun${ext}" ./cmd/whatgate

	# Ship docs alongside the binaries.
	cp README.md "$stage/" 2>/dev/null || true
	[ -f LICENSE ] && cp LICENSE "$stage/"

	# Package: zip for Windows, tar.gz elsewhere.
	base="whatgate_${VERSION}_${GOOS}_${GOARCH}"
	if [ "$GOOS" = "windows" ]; then
		( cd "$DIST" && zip -qr "${base}.zip" "$base" )
	else
		( cd "$DIST" && tar -czf "${base}.tar.gz" "$base" )
	fi
	rm -rf "$stage"
done

# Checksums for every archive, so downloaders can verify integrity.
( cd "$DIST" && sha256sum whatgate_* > SHA256SUMS )

echo
echo "Artifacts in dist/:"
ls -1 "$DIST"
