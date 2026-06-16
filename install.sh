#!/usr/bin/env sh
set -eu

REPO="${REPO:-bobyasasas/kai-systemctl}"
BIN="${BIN:-kai-systemctl}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

cmd="${1:-install}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

as_root_install() {
  src="$1"
  dst="$2"
  if [ "$(id -u)" -eq 0 ]; then
    install -m 0755 "$src" "$dst"
  else
    need sudo
    sudo install -m 0755 "$src" "$dst"
  fi
}

as_root_remove() {
  path="$1"
  if [ "$(id -u)" -eq 0 ]; then
    rm -f "$path"
  else
    need sudo
    sudo rm -f "$path"
  fi
}

detect_platform() {
  need uname
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
  esac
}

latest_version() {
  need curl
  url="$(curl -fsSIL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")"
  tag="${url##*/}"
  if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
    echo "failed to detect latest version" >&2
    exit 1
  fi
  printf '%s\n' "$tag"
}

installed_path() {
  if command -v "$BIN" >/dev/null 2>&1; then
    command -v "$BIN"
  elif [ -x "$INSTALL_DIR/$BIN" ]; then
    printf '%s\n' "$INSTALL_DIR/$BIN"
  else
    printf '%s\n' "$INSTALL_DIR/$BIN"
  fi
}

installed_version() {
  path="$(installed_path)"
  if [ -x "$path" ]; then
    "$path" version 2>/dev/null || printf 'unknown\n'
  else
    printf 'not installed\n'
  fi
}

install_version() {
  version="$1"
  detect_platform
  need curl
  need tar

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  url="https://github.com/${REPO}/releases/download/${version}/${BIN}_${OS}_${ARCH}.tar.gz"
  echo "downloading $url"
  curl -fsSL "$url" -o "$tmp_dir/${BIN}.tar.gz"
  tar -xzf "$tmp_dir/${BIN}.tar.gz" -C "$tmp_dir"
  as_root_install "$tmp_dir/$BIN" "$INSTALL_DIR/$BIN"
  echo "installed $BIN $version to $INSTALL_DIR/$BIN"
}

status() {
  latest="$(latest_version)"
  current="$(installed_version)"
  path="$(installed_path)"

  echo "binary: $BIN"
  echo "path: $path"
  echo "installed: $current"
  echo "latest: $latest"
  if [ "$current" = "$latest" ]; then
    echo "status: up to date"
  elif [ "$current" = "not installed" ]; then
    echo "status: not installed"
  else
    echo "status: upgrade available"
  fi
}

install_cmd() {
  latest="$(latest_version)"
  current="$(installed_version)"
  if [ "$current" = "$latest" ]; then
    echo "$BIN $current is already installed"
    return 0
  fi
  install_version "$latest"
}

upgrade_cmd() {
  latest="$(latest_version)"
  current="$(installed_version)"
  if [ "$current" = "$latest" ]; then
    echo "$BIN is already up to date ($current)"
    return 0
  fi
  install_version "$latest"
}

uninstall_cmd() {
  path="$(installed_path)"
  if [ ! -e "$path" ]; then
    echo "$BIN is not installed"
    return 0
  fi
  as_root_remove "$path"
  echo "removed $path"
}

usage() {
  cat <<EOF
kai-systemctl installer

Usage:
  sh install.sh [install|status|upgrade|uninstall]

Environment:
  REPO=bobyasasas/kai-systemctl
  INSTALL_DIR=/usr/local/bin
  BIN=kai-systemctl
EOF
}

case "$cmd" in
  install) install_cmd ;;
  status) status ;;
  upgrade) upgrade_cmd ;;
  uninstall|remove) uninstall_cmd ;;
  help|-h|--help) usage ;;
  *) echo "unknown command: $cmd" >&2; usage; exit 1 ;;
esac
