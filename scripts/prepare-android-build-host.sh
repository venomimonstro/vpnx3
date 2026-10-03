#!/usr/bin/env bash
set -euo pipefail

GRADLE_URL=""
GRADLE_SHA256=""
ANDROID_TOOLS_URL=""
ANDROID_TOOLS_SHA256=""
SDK_PACKAGES="platform-tools,platforms;android-37"
ANDROID_SDK_ROOT="/opt/android-sdk"
GRADLE_ROOT="/opt/gradle"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --gradle-url) GRADLE_URL="${2:-}"; shift 2;;
    --gradle-sha256) GRADLE_SHA256="${2:-}"; shift 2;;
    --android-tools-url) ANDROID_TOOLS_URL="${2:-}"; shift 2;;
    --android-tools-sha256) ANDROID_TOOLS_SHA256="${2:-}"; shift 2;;
    --sdk-packages) SDK_PACKAGES="${2:-}"; shift 2;;
    --android-sdk-root) ANDROID_SDK_ROOT="${2:-}"; shift 2;;
    --gradle-root) GRADLE_ROOT="${2:-}"; shift 2;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -n "$GRADLE_URL" && -n "$GRADLE_SHA256" && -n "$ANDROID_TOOLS_URL" && -n "$ANDROID_TOOLS_SHA256" ]] || {
  echo "gradle-url, gradle-sha256, android-tools-url and android-tools-sha256 are required" >&2
  exit 2
}

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends   ca-certificates curl unzip zip git openjdk-17-jdk-headless
rm -rf /var/lib/apt/lists/*

download_verified() {
  local url="$1" expected="$2" dest="$3"
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$dest" "$url"
  local actual
  actual="$(sha256sum "$dest" | awk '{print $1}')"
  [[ "${actual,,}" == "${expected,,}" ]] || {
    echo "SHA-256 mismatch for $url" >&2
    rm -f "$dest"
    exit 1
  }
}

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

download_verified "$GRADLE_URL" "$GRADLE_SHA256" "$tmpdir/gradle.zip"
rm -rf "$GRADLE_ROOT"
install -d -m 0755 "$GRADLE_ROOT"
unzip -q "$tmpdir/gradle.zip" -d "$tmpdir/gradle"
gradle_home="$(find "$tmpdir/gradle" -mindepth 1 -maxdepth 1 -type d | head -n1)"
[[ -n "$gradle_home" && -x "$gradle_home/bin/gradle" ]] || {
  echo "Invalid Gradle distribution" >&2
  exit 1
}
cp -a "$gradle_home/." "$GRADLE_ROOT/"
ln -sfn "$GRADLE_ROOT/bin/gradle" /usr/local/bin/gradle

download_verified "$ANDROID_TOOLS_URL" "$ANDROID_TOOLS_SHA256" "$tmpdir/android-tools.zip"
rm -rf "$ANDROID_SDK_ROOT/cmdline-tools/latest"
install -d -m 0755 "$ANDROID_SDK_ROOT/cmdline-tools"
unzip -q "$tmpdir/android-tools.zip" -d "$tmpdir/android"
tools_dir="$(find "$tmpdir/android" -type f -name sdkmanager -printf '%h\n' | head -n1)"
[[ -n "$tools_dir" ]] || { echo "sdkmanager not found in Android tools archive" >&2; exit 1; }
tools_root="$(dirname "$tools_dir")"
install -d -m 0755 "$ANDROID_SDK_ROOT/cmdline-tools/latest"
cp -a "$tools_root/." "$ANDROID_SDK_ROOT/cmdline-tools/latest/"

SDKMANAGER="$ANDROID_SDK_ROOT/cmdline-tools/latest/bin/sdkmanager"
[[ -x "$SDKMANAGER" ]] || { echo "sdkmanager is not executable" >&2; exit 1; }

export ANDROID_SDK_ROOT
export ANDROID_HOME="$ANDROID_SDK_ROOT"
yes | "$SDKMANAGER" --licenses >/dev/null || true

IFS=',' read -r -a packages <<<"$SDK_PACKAGES"
clean_packages=()
for pkg in "${packages[@]}"; do
  pkg="$(echo "$pkg" | xargs)"
  [[ -n "$pkg" ]] && clean_packages+=("$pkg")
done
[[ "${#clean_packages[@]}" -gt 0 ]] || { echo "No Android SDK packages requested" >&2; exit 1; }
"$SDKMANAGER" "${clean_packages[@]}"

[[ -f "$ANDROID_SDK_ROOT/platforms/android-37/android.jar" ]] || {
  echo "compileSdk 37 platform was not installed; include platforms;android-37 in --sdk-packages" >&2
  exit 1
}

java -version
gradle --version
"$SDKMANAGER" --list_installed

echo
echo "Android build host prepared."
echo "ANDROID_SDK_ROOT=$ANDROID_SDK_ROOT"
echo "Gradle=$GRADLE_ROOT/bin/gradle"
echo "Next: install vpnx3-build-worker and provide release keystore secrets."
