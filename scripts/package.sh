#!/usr/bin/env sh
set -eu

VERSION="${VERSION:-dev}"
DEB_VERSION="$VERSION"
case "$DEB_VERSION" in
  [0-9]*) ;;
  *) DEB_VERSION="0.0.0-$DEB_VERSION" ;;
esac
DIST="${DIST:-dist}"
PREFIX="${PREFIX:-/usr/local}"

mkdir -p "$DIST/packages"

for bin in "$DIST"/dockan-linux-*; do
  [ -f "$bin" ] || continue
  name="$(basename "$bin")"
  archive="$DIST/packages/${name}-${VERSION}.tar.gz"
  plain_archive="$DIST/packages/${name}.tar.gz"
  tmp="$DIST/packages/${name}-${VERSION}"
  rm -rf "$tmp"
  mkdir -p "$tmp/bin"
  cp "$bin" "$tmp/bin/dockan"
  chmod 0755 "$tmp/bin/dockan"
  tar -C "$tmp" -czf "$archive" .
  cp "$archive" "$plain_archive"
  rm -rf "$tmp"
  echo "$archive"
  echo "$plain_archive"
done

if command -v dpkg-deb >/dev/null 2>&1 && [ -f "$DIST/dockan-linux-amd64" ]; then
  pkg="$DIST/packages/dockan_${DEB_VERSION}_amd64"
  rm -rf "$pkg"
  mkdir -p "$pkg/DEBIAN" "$pkg$PREFIX/bin"
  cp "$DIST/dockan-linux-amd64" "$pkg$PREFIX/bin/dockan"
  chmod 0755 "$pkg$PREFIX/bin/dockan"
  cat > "$pkg/DEBIAN/control" <<EOF
Package: dockan
Version: $DEB_VERSION
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Dockan
Description: Local-first container tool
EOF
  dpkg-deb --root-owner-group --build "$pkg" "$DIST/packages/dockan_${DEB_VERSION}_amd64.deb"
  cp "$DIST/packages/dockan_${DEB_VERSION}_amd64.deb" "$DIST/packages/dockan_amd64.deb"
  rm -rf "$pkg"
fi

if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "$DIST/packages"
    sha256sum * > SHA256SUMS
  )
fi
