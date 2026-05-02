#!/usr/bin/env sh
set -eu

REPO="${REPO:-Dockan-Conteneurisation-libre/Dockan}"
VERSION="${VERSION:-latest}"
PREFIX_WAS_SET="${PREFIX+x}"
BINDIR_WAS_SET="${BINDIR+x}"
PREFIX="${PREFIX:-/usr/local}"
BINDIR="${BINDIR:-$PREFIX/bin}"
INSTALL_SOURCE="${INSTALL_SOURCE:-auto}"
GO_BIN="${GO:-go}"
TMPDIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMPDIR"
}
trap cleanup EXIT INT TERM

info() {
  printf '%s\n' "$*"
}

fail() {
  printf 'Erreur: %s\n' "$*" >&2
  exit 1
}

detect_platform() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$os" in
    linux) ;;
    *) fail "Dockan supporte Linux pour le moment, pas $os" ;;
  esac
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    armv7l|armv7*) arch="armv7" ;;
    i386|i686) arch="386" ;;
    riscv64) arch="riscv64" ;;
    *) fail "architecture non supportée: $arch" ;;
  esac
  printf '%s-%s' "$os" "$arch"
}

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi
  fail "curl ou wget est requis pour télécharger Dockan"
}

verify_checksum() {
  file="$1"
  asset="$2"
  sums="$TMPDIR/SHA256SUMS"
  if ! download "$(release_url SHA256SUMS)" "$sums"; then
    info "Checksum absent sur la release, installation sans vérification SHA256."
    return 0
  fi
  if ! command -v sha256sum >/dev/null 2>&1; then
    info "sha256sum absent, installation sans vérification SHA256."
    return 0
  fi
  expected="$(grep "  $asset\$" "$sums" | awk '{print $1}' || true)"
  [ -n "$expected" ] || fail "checksum introuvable pour $asset"
  actual="$(sha256sum "$file" | awk '{print $1}')"
  [ "$actual" = "$expected" ] || fail "checksum invalide pour $asset"
}

release_url() {
  asset="$1"
  if [ "$VERSION" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/%s' "$REPO" "$asset"
  else
    printf 'https://github.com/%s/releases/download/%s/%s' "$REPO" "$VERSION" "$asset"
  fi
}

install_file() {
  src="$1"
  choose_bindir
  mkdir -p "$BINDIR" || fail "impossible de créer $BINDIR"
  tmp="$BINDIR/.dockan.tmp.$$"
  rm -f "$tmp"
  cp "$src" "$tmp" || fail "impossible de préparer l'installation dans $BINDIR. Essayez: curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | sudo sh"
  chmod 0755 "$tmp" || {
    rm -f "$tmp"
    fail "impossible de rendre $tmp exécutable"
  }
  mv -f "$tmp" "$BINDIR/dockan" || {
    rm -f "$tmp"
    fail "impossible d'installer dans $BINDIR. Essayez: curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | sudo sh"
  }
}

choose_bindir() {
  if can_write_bindir "$BINDIR"; then
    return 0
  fi

  if [ -z "$PREFIX_WAS_SET" ] && [ -z "$BINDIR_WAS_SET" ] && [ "$BINDIR" = "/usr/local/bin" ]; then
    [ -n "${HOME:-}" ] || fail "/usr/local/bin non accessible et HOME est vide"
    BINDIR="$HOME/.local/bin"
    info "/usr/local/bin n'est pas accessible sans sudo; installation utilisateur dans $BINDIR."
    return 0
  fi

  fail "$BINDIR n'est pas accessible en écriture"
}

can_write_bindir() {
  dir="$1"
  if [ -d "$dir" ]; then
    [ -w "$dir" ]
    return
  fi
  parent="$(dirname "$dir")"
  [ -d "$parent" ] && [ -w "$parent" ]
}

install_from_release() {
  platform="$(detect_platform)"
  archive="dockan-$platform.tar.gz"
  binary="dockan-$platform"

  if download "$(release_url "$archive")" "$TMPDIR/dockan.tar.gz"; then
    verify_checksum "$TMPDIR/dockan.tar.gz" "$archive"
    mkdir -p "$TMPDIR/unpack"
    tar -xzf "$TMPDIR/dockan.tar.gz" -C "$TMPDIR/unpack"
    if [ -f "$TMPDIR/unpack/bin/dockan" ]; then
      install_file "$TMPDIR/unpack/bin/dockan"
      return 0
    fi
    if [ -f "$TMPDIR/unpack/dockan" ]; then
      install_file "$TMPDIR/unpack/dockan"
      return 0
    fi
    fail "archive release invalide: $archive"
  fi

  if download "$(release_url "$binary")" "$TMPDIR/dockan"; then
    verify_checksum "$TMPDIR/dockan" "$binary"
    install_file "$TMPDIR/dockan"
    return 0
  fi

  return 1
}

install_from_source() {
  if [ ! -f "./cmd/dockan.go" ]; then
    return 1
  fi
  command -v "$GO_BIN" >/dev/null 2>&1 || fail "Go est requis pour construire Dockan depuis les sources"
  CGO_ENABLED=0 "$GO_BIN" build -trimpath -ldflags "-s -w" -o "$TMPDIR/dockan" ./cmd/dockan.go
  install_file "$TMPDIR/dockan"
}

case "$INSTALL_SOURCE" in
  release)
    install_from_release || fail "release introuvable pour $(detect_platform)"
    ;;
  source)
    install_from_source || fail "lancez ce script depuis le dépôt Dockan"
    ;;
  auto)
    if ! install_from_release; then
      info "Release introuvable, tentative de build local..."
      install_from_source || fail "impossible d'installer: release absente et sources locales introuvables"
    fi
    ;;
  *)
    fail "INSTALL_SOURCE doit être auto, release ou source"
    ;;
esac

info "Dockan installé dans $BINDIR/dockan"
case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *)
    info "Note: $BINDIR n'est pas dans PATH."
    info "Ajoutez-le avec: export PATH=\"$BINDIR:\$PATH\""
    ;;
esac
info "Essayez: dockan doctor"
