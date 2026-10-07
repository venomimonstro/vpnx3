#!/usr/bin/env bash
set -euo pipefail

APK=""
AAB=""
SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apk) APK="${2:-}"; shift 2;;
    --aab) AAB="${2:-}"; shift 2;;
    -h|--help)
      echo "Usage: validate-android-release.sh [--apk app.apk] [--aab app.aab]"
      exit 0;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done

[[ -n "$APK" || -n "$AAB" ]] || { echo "Provide --apk or --aab" >&2; exit 2; }
[[ -n "$SDK_ROOT" ]] || { echo "ANDROID_SDK_ROOT/ANDROID_HOME is required" >&2; exit 2; }

find_tool(){
  local name="$1"
  find "$SDK_ROOT/build-tools" -type f -name "$name" -print 2>/dev/null | sort -V | tail -1
}

APKSIGNER="$(find_tool apksigner)"
AAPT2="$(find_tool aapt2)"
[[ -x "$APKSIGNER" ]] || { echo "apksigner not found" >&2; exit 1; }
[[ -x "$AAPT2" ]] || { echo "aapt2 not found" >&2; exit 1; }

if [[ -n "$APK" ]]; then
  [[ -f "$APK" ]] || { echo "APK missing: $APK" >&2; exit 1; }
  "$APKSIGNER" verify --verbose --print-certs "$APK"
  manifest="$("$AAPT2" dump xmltree "$APK" AndroidManifest.xml)"
  grep -q 'android.permission.INTERNET' <<<"$manifest" || { echo "INTERNET permission missing" >&2; exit 1; }
  grep -q 'A: android:allowBackup(.*)=.*0x0' <<<"$manifest" || { echo "allowBackup must be false" >&2; exit 1; }
  grep -q 'A: android:usesCleartextTraffic(.*)=.*0x0' <<<"$manifest" || { echo "usesCleartextTraffic must be false" >&2; exit 1; }
  if grep -Eq 'android.permission.(READ_CONTACTS|READ_SMS|READ_CALL_LOG|ACCESS_FINE_LOCATION|RECORD_AUDIO|CAMERA)' <<<"$manifest"; then
    echo "Unexpected privacy-sensitive permission found" >&2
    exit 1
  fi
  echo "[OK] APK signature and security manifest validated"
fi

if [[ -n "$AAB" ]]; then
  [[ -f "$AAB" ]] || { echo "AAB missing: $AAB" >&2; exit 1; }
  command -v jarsigner >/dev/null 2>&1 || { echo "jarsigner is required" >&2; exit 1; }
  jarsigner -verify -strict -certs "$AAB"
  echo "[OK] AAB JAR signature validated"
fi
