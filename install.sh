#!/usr/bin/env sh
set -eu

REPO="${REPO:-bobyasasas/kai-systemctl}"
BIN="${BIN:-kai-systemctl}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need uname
need curl
need tar

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

URL="https://github.com/${REPO}/releases/latest/download/${BIN}_${OS}_${ARCH}.tar.gz"
echo "downloading $URL"
curl -fsSL "$URL" -o "$TMP_DIR/${BIN}.tar.gz"
tar -xzf "$TMP_DIR/${BIN}.tar.gz" -C "$TMP_DIR"

if [ "$(id -u)" -eq 0 ]; then
  install -m 0755 "$TMP_DIR/$BIN" "$INSTALL_DIR/$BIN"
else
  sudo install -m 0755 "$TMP_DIR/$BIN" "$INSTALL_DIR/$BIN"
fi

echo "installed $BIN to $INSTALL_DIR/$BIN"
echo "try: $BIN list"
