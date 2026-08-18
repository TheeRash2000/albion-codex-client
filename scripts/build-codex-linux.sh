#!/usr/bin/env bash
#
# Build the Codex Client for Linux (CGO + libpcap).
#
# Counterpart to scripts/build-codex.ps1 (Windows). Linux links libpcap via
# cgo, so you need the headers first:
#
#   sudo apt-get install -y libpcap-dev        # Debian/Ubuntu
#   sudo dnf install -y libpcap-devel          # Fedora
#
# Usage:
#   ./scripts/build-codex-linux.sh [VERSION]
#
# VERSION is embedded as main.version (shown by -version). Default: dev.
#
# Output: codex-client-linux in the repo root, plus its sha256.
#
# NOTE ON AUTO-UPDATE: do NOT point Linux builds at the Windows version.json -
# its url/sha256 reference codex-client.exe. Auto-update is disabled on
# non-Windows in codex_update.go, and must stay that way until a per-OS
# manifest exists (a self-update would also strip the setcap capability below).
#
# RUNNING: packet capture needs CAP_NET_RAW. Grant it once after each build:
#   sudo setcap cap_net_raw,cap_net_admin=eip ./codex-client-linux
# (or run under a systemd unit with AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN)

set -euo pipefail

VERSION="${1:-dev}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="codex-client-linux"

cd "$REPO"

if ! command -v go >/dev/null; then
  echo "error: Go toolchain not found" >&2
  exit 1
fi

if ! ldconfig -p 2>/dev/null | grep -q libpcap; then
  echo "error: libpcap not found - install libpcap-dev (Debian/Ubuntu) or libpcap-devel (Fedora)" >&2
  exit 1
fi

echo "Go:      $(go version)"
echo "Version: $VERSION"

CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=$VERSION" -o "$OUT" albiondata-client.go

# Older distros ship libpcap.so.0.8, newer ones libpcap.so.1; loosen the soname
# so one binary runs on both (same trick as upstream scripts/build-linux.sh).
if command -v patchelf >/dev/null; then
  patchelf --replace-needed libpcap.so.0.8 libpcap.so "$OUT" 2>/dev/null || true
  patchelf --replace-needed libpcap.so.1 libpcap.so "$OUT" 2>/dev/null || true
fi

SIZE=$(du -m "$OUT" | cut -f1)
SHA=$(sha256sum "$OUT" | cut -d' ' -f1)
echo "Built $OUT (${SIZE} MB)"
echo "sha256: $SHA"
echo
echo "Next steps:"
echo "  sudo setcap cap_net_raw,cap_net_admin=eip ./$OUT"
echo "  ./$OUT -version"
