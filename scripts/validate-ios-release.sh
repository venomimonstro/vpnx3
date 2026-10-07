#!/bin/bash
set -euo pipefail

IPA=""
EXPECTED_VERSION=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --ipa) IPA="${2:-}"; shift 2;;
    --version) EXPECTED_VERSION="${2:-}"; shift 2;;
    -h|--help) echo "Usage: validate-ios-release.sh --ipa file.ipa --version 1.2.3"; exit 0;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done

[[ "$(uname -s)" == "Darwin" ]] || { echo "macOS required" >&2; exit 1; }
[[ -f "$IPA" ]] || { echo "IPA missing" >&2; exit 1; }
[[ -n "$EXPECTED_VERSION" ]] || { echo "version required" >&2; exit 2; }
for cmd in codesign unzip plutil find grep mktemp; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing $cmd" >&2; exit 1; }
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
unzip -q "$IPA" -d "$tmp"

app_count="$(find "$tmp/Payload" -maxdepth 1 -type d -name '*.app' | wc -l | tr -d ' ')"
[[ "$app_count" == "1" ]] || { echo "Expected exactly one .app in IPA" >&2; exit 1; }
APP="$(find "$tmp/Payload" -maxdepth 1 -type d -name '*.app' -print)"

codesign --verify --deep --strict --verbose=2 "$APP"

APP_ID="$(plutil -extract CFBundleIdentifier raw "$APP/Info.plist")"
APP_VERSION="$(plutil -extract CFBundleShortVersionString raw "$APP/Info.plist")"
[[ "$APP_ID" == "ru.vpnx3.app" ]] || { echo "Unexpected app bundle id: $APP_ID" >&2; exit 1; }
[[ "$APP_VERSION" == "$EXPECTED_VERSION" ]] || { echo "Unexpected app version: $APP_VERSION" >&2; exit 1; }

EXT="$APP/PlugIns/PacketTunnel.appex"
[[ -d "$EXT" ]] || { echo "PacketTunnel extension missing" >&2; exit 1; }
codesign --verify --strict --verbose=2 "$EXT"
EXT_ID="$(plutil -extract CFBundleIdentifier raw "$EXT/Info.plist")"
[[ "$EXT_ID" == "ru.vpnx3.app.PacketTunnel" ]] || { echo "Unexpected extension bundle id: $EXT_ID" >&2; exit 1; }

APP_ENT="$tmp/app-entitlements.plist"
EXT_ENT="$tmp/ext-entitlements.plist"
codesign -d --entitlements "$APP_ENT" "$APP" 2>/dev/null
codesign -d --entitlements "$EXT_ENT" "$EXT" 2>/dev/null

grep -q "packet-tunnel-provider" "$EXT_ENT" || {
  echo "Packet Tunnel network extension entitlement missing" >&2; exit 1;
}
grep -q "ru.vpnx3.shared" "$APP_ENT" || {
  echo "App shared Keychain access group missing" >&2; exit 1;
}
grep -q "ru.vpnx3.shared" "$EXT_ENT" || {
  echo "Extension shared Keychain access group missing" >&2; exit 1;
}
APP_GROUP="$(plutil -extract VPNX3KeychainAccessGroup raw "$APP/Info.plist" 2>/dev/null || true)"
EXT_GROUP="$(plutil -extract VPNX3KeychainAccessGroup raw "$EXT/Info.plist" 2>/dev/null || true)"
[[ -n "$APP_GROUP" && "$APP_GROUP" == "$EXT_GROUP" && "$APP_GROUP" == *".ru.vpnx3.shared" ]] || {
  echo "App/extension Keychain access group mismatch" >&2; exit 1;
}

if plutil -extract get-task-allow raw "$APP_ENT" >/dev/null 2>&1; then
  debug="$(plutil -extract get-task-allow raw "$APP_ENT" 2>/dev/null || true)"
  [[ "$debug" != "true" ]] || { echo "Distribution app contains get-task-allow=true" >&2; exit 1; }
fi
if plutil -extract get-task-allow raw "$EXT_ENT" >/dev/null 2>&1; then
  debug="$(plutil -extract get-task-allow raw "$EXT_ENT" 2>/dev/null || true)"
  [[ "$debug" != "true" ]] || { echo "Distribution extension contains get-task-allow=true" >&2; exit 1; }
fi

if grep -R -a -E 'http://(localhost|127\.0\.0\.1)|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY' "$APP" >/dev/null 2>&1; then
  echo "Development URL or private key material found in IPA" >&2
  exit 1
fi

echo "[OK] iOS IPA signature, bundle IDs, version and Packet Tunnel entitlements validated"
