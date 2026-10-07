#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IOS="$ROOT/apps/ios"
DERIVED="$IOS/.derived"
ARCHIVE="$IOS/.archive/VPNX3.xcarchive"
EXPORT_DIR="$IOS/.export"

: "${VPNX3_IOS_TEAM_ID:?VPNX3_IOS_TEAM_ID is required}"
: "${VPNX3_IOS_EXPORT_OPTIONS_PLIST:?VPNX3_IOS_EXPORT_OPTIONS_PLIST is required}"
: "${VPNX3_CLIENT_CONTROL_URL:?VPNX3_CLIENT_CONTROL_URL is required}"
: "${VPNX3_CONFIG_PUBLIC_KEY:?VPNX3_CONFIG_PUBLIC_KEY is required}"
: "${VPNX3_RELEASE_VERSION:?VPNX3_RELEASE_VERSION is required}"

for cmd in xcodegen xcodebuild xcrun go make git python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required command: $cmd" >&2; exit 1; }
done
[[ -f "$VPNX3_IOS_EXPORT_OPTIONS_PLIST" ]] || { echo "ExportOptions.plist missing" >&2; exit 1; }
[[ "$VPNX3_CLIENT_CONTROL_URL" == https://* ]] || { echo "Control URL must use https" >&2; exit 1; }

version="$(python3 - "$VPNX3_RELEASE_VERSION" <<'PY'
import re,sys
v=sys.argv[1].strip().lstrip("v").split("-",1)[0]
if not re.fullmatch(r"\d+(?:\.\d+){0,2}",v): raise SystemExit("invalid iOS version")
p=[int(x) for x in v.split(".")]
while len(p)<3:p.append(0)
if any(x>999 for x in p[1:]) or p[0]>2100: raise SystemExit("version out of range")
print(v, p[0]*1000000+p[1]*1000+p[2])
PY
)"
MARKETING_VERSION="${version% *}"
CURRENT_PROJECT_VERSION="${version#* }"

rm -rf "$DERIVED" "$IOS/.archive" "$EXPORT_DIR" "$IOS/VPNX3.xcodeproj"
mkdir -p "$DERIVED" "$IOS/.archive" "$EXPORT_DIR"

cd "$IOS"
xcodegen generate --spec project.yml

/usr/libexec/PlistBuddy -c "Set :VPNX3ControlURL $VPNX3_CLIENT_CONTROL_URL" VPNX3/Info.plist 2>/dev/null ||
  /usr/libexec/PlistBuddy -c "Add :VPNX3ControlURL string $VPNX3_CLIENT_CONTROL_URL" VPNX3/Info.plist
/usr/libexec/PlistBuddy -c "Set :VPNX3ConfigPublicKey $VPNX3_CONFIG_PUBLIC_KEY" VPNX3/Info.plist 2>/dev/null ||
  /usr/libexec/PlistBuddy -c "Add :VPNX3ConfigPublicKey string $VPNX3_CONFIG_PUBLIC_KEY" VPNX3/Info.plist

xcodebuild -resolvePackageDependencies   -project VPNX3.xcodeproj -scheme VPNX3 -derivedDataPath "$DERIVED"

WG="$DERIVED/SourcePackages/checkouts/wireguard-apple/Sources/WireGuardKitGo"
[[ -d "$WG" ]] || { echo "Pinned WireGuardKit checkout missing" >&2; exit 1; }
actual="$(git -C "$DERIVED/SourcePackages/checkouts/wireguard-apple" rev-parse HEAD)"
[[ "$actual" == "2fec12a6e1f6e3460b6ee483aa00ad29cddadab1" ]] || {
  echo "Unexpected WireGuard Apple revision: $actual" >&2; exit 1;
}

PRODUCTS="$DERIVED/Build/Products/Release-iphoneos"
mkdir -p "$PRODUCTS"
make -C "$WG"   ARCHS=arm64   PLATFORM_NAME=iphoneos   SDKROOT="$(xcrun --sdk iphoneos --show-sdk-path)"   CONFIGURATION_BUILD_DIR="$PRODUCTS"   CONFIGURATION_TEMP_DIR="$DERIVED/Build/Intermediates.noindex/WireGuardGo"   build version-header

xcodebuild archive   -project VPNX3.xcodeproj   -scheme VPNX3   -configuration Release   -derivedDataPath "$DERIVED"   -archivePath "$ARCHIVE"   -destination "generic/platform=iOS"   DEVELOPMENT_TEAM="$VPNX3_IOS_TEAM_ID"   MARKETING_VERSION="$MARKETING_VERSION"   CURRENT_PROJECT_VERSION="$CURRENT_PROJECT_VERSION"

xcodebuild -exportArchive   -archivePath "$ARCHIVE"   -exportPath "$EXPORT_DIR"   -exportOptionsPlist "$VPNX3_IOS_EXPORT_OPTIONS_PLIST"

mapfile -t ipas < <(find "$EXPORT_DIR" -maxdepth 1 -type f -name '*.ipa' -print)
[[ "${#ipas[@]}" -eq 1 ]] || { echo "Expected exactly one IPA" >&2; exit 1; }
printf '%s\n' "${ipas[0]}"
