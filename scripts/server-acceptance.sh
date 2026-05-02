#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
DOCKAN_BIN="${DOCKAN_BIN:-$ROOT/dockan}"

if [ ! -x "$DOCKAN_BIN" ]; then
  printf 'Dockan binary not found at %s\n' "$DOCKAN_BIN" >&2
  printf 'Build it first with: make build\n' >&2
  exit 1
fi

STORE="$(mktemp -d)"

cleanup() {
  DOCKAN_HOME="$STORE" "$DOCKAN_BIN" compose down -f "$ROOT/examples/compose/dockan.yml" >/dev/null 2>&1 || true
  rm -rf "$STORE"
}
trap cleanup EXIT INT TERM

export DOCKAN_HOME="$STORE"

printf '== Dockan server acceptance smoke test ==\n'
printf 'Using DOCKAN_HOME=%s\n' "$DOCKAN_HOME"

printf '\n== Doctor ==\n'
"$DOCKAN_BIN" doctor || true

printf '\n== Build and local run ==\n'
"$DOCKAN_BIN" build -t hello:stable "$ROOT/examples/hello"
"$DOCKAN_BIN" tag hello:stable hello:previous
"$DOCKAN_BIN" run hello:stable

printf '\n== Compose up ==\n'
"$DOCKAN_BIN" compose up -f "$ROOT/examples/compose/dockan.yml"
"$DOCKAN_BIN" ps -a

printf '\n== Compose redeploy ==\n'
"$DOCKAN_BIN" compose redeploy -f "$ROOT/examples/compose/dockan.yml"
"$DOCKAN_BIN" ps -a

printf '\n== Rollback tag check ==\n'
"$DOCKAN_BIN" tag hello:previous hello:stable
"$DOCKAN_BIN" images

if [ "${DOCKAN_ACCEPT_BRIDGE:-0}" = "1" ]; then
  if [ "$(id -u)" != "0" ]; then
    printf '\nSkipping bridge/NAT check: run as root or sudo with DOCKAN_ACCEPT_BRIDGE=1.\n'
  else
    printf '\n== Bridge/NAT check ==\n'
    "$DOCKAN_BIN" network create prodnet --driver bridge --subnet 10.89.0.0/24 --gateway 10.89.0.1/24 --bridge dockan0 || true
    "$DOCKAN_BIN" network enable prodnet
    "$DOCKAN_BIN" network hosts prodnet
    "$DOCKAN_BIN" network disable prodnet
  fi
fi

printf '\nAcceptance smoke test completed.\n'
