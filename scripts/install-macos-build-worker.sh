#!/bin/bash
set -euo pipefail

BINARY_URL=""
BINARY_SHA256=""
CONTROL_URL=""
ENROLLMENT_TOKEN=""
NODE_NAME="macos-build-worker"
SOURCE_REPO="https://github.com/venomimonstro/vpnx3.git"
TARGETS="ios_ipa"
CLIENT_CONTROL_URL=""
CONFIG_PUBLIC_KEY=""
RELEASE_PUBLIC_KEY=""
IOS_TEAM_ID=""
IOS_EXPORT_OPTIONS_PLIST=""
WORK_ROOT="$HOME/Library/Application Support/VPNX3BuildWorker"
INSTALL_DIR="/usr/local/bin"
PLIST_PATH="$HOME/Library/LaunchAgents/ru.vpnx3.build-worker.plist"

usage(){
cat <<'EOF'
VPNX3 macOS Build Worker installer.

Required:
  --binary-url https://...
  --sha256 SHA256
  --control-url https://...
  --enrollment-token TOKEN

Optional:
  --node-name macos-build-worker
  --source-repo https://github.com/venomimonstro/vpnx3.git
  --targets ios_ipa
  --client-control-url https://...
  --config-public-key ...
  --release-public-key ...
  --ios-team-id TEAMID
  --ios-export-options-plist /secure/path/ExportOptions.plist

Requirements:
- Apple Silicon Mac (arm64);
- Xcode selected through xcode-select;
- git, xcodebuild, codesign and security available;
- signing identities/provisioning profiles are installed in the macOS user Keychain.

The script does not import certificates or provisioning profiles.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary-url) BINARY_URL="${2:-}"; shift 2;;
    --sha256) BINARY_SHA256="${2:-}"; shift 2;;
    --control-url) CONTROL_URL="${2:-}"; shift 2;;
    --enrollment-token) ENROLLMENT_TOKEN="${2:-}"; shift 2;;
    --node-name) NODE_NAME="${2:-}"; shift 2;;
    --source-repo) SOURCE_REPO="${2:-}"; shift 2;;
    --targets) TARGETS="${2:-}"; shift 2;;
    --client-control-url) CLIENT_CONTROL_URL="${2:-}"; shift 2;;
    --config-public-key) CONFIG_PUBLIC_KEY="${2:-}"; shift 2;;
    --release-public-key) RELEASE_PUBLIC_KEY="${2:-}"; shift 2;;
    --ios-team-id) IOS_TEAM_ID="${2:-}"; shift 2;;
    --ios-export-options-plist) IOS_EXPORT_OPTIONS_PLIST="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2;;
  esac
done

[[ "$(uname -s)" == "Darwin" ]] || { echo "macOS is required" >&2; exit 1; }
[[ "$(uname -m)" == "arm64" ]] || { echo "Apple Silicon arm64 is required for this worker binary" >&2; exit 1; }
[[ "$CONTROL_URL" == https://* ]] || { echo "control-url must use https" >&2; exit 2; }
[[ "$BINARY_URL" == https://* ]] || { echo "binary-url must use https" >&2; exit 2; }
[[ -n "$BINARY_SHA256" && -n "$ENROLLMENT_TOKEN" ]] || { echo "sha256 and enrollment-token are required" >&2; exit 2; }
if [[ "$TARGETS" == *"ios_ipa"* ]]; then
  [[ -n "$IOS_TEAM_ID" ]] || { echo "ios-team-id is required for ios_ipa" >&2; exit 2; }
  [[ -f "$IOS_EXPORT_OPTIONS_PLIST" ]] || { echo "local ExportOptions.plist is required for ios_ipa" >&2; exit 2; }
fi

for cmd in curl shasum git xcodebuild xcode-select codesign security launchctl xcodegen go make python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

xcode_path="$(xcode-select -p 2>/dev/null || true)"
[[ "$xcode_path" == *"/Xcode.app/"* || "$xcode_path" == *"/Developer"* ]] || {
  echo "Full Xcode must be selected with xcode-select" >&2
  exit 1
}
xcodebuild -version

mkdir -p "$WORK_ROOT/work" "$HOME/Library/LaunchAgents"
chmod 700 "$WORK_ROOT"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$tmp" "$BINARY_URL"
actual="$(shasum -a 256 "$tmp" | awk '{print $1}')"
actual_lc="$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')"
expected_lc="$(printf '%s' "$BINARY_SHA256" | tr '[:upper:]' '[:lower:]')"
[[ "$actual_lc" == "$expected_lc" ]] || {
  echo "SHA-256 mismatch" >&2
  exit 1
}

sudo install -m 0755 "$tmp" "$INSTALL_DIR/vpnx3-build-worker"

cat >"$WORK_ROOT/worker.env" <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_ENROLLMENT_TOKEN=$ENROLLMENT_TOKEN
VPNX3_NODE_NAME=$NODE_NAME
VPNX3_SOURCE_REPO=$SOURCE_REPO
VPNX3_BUILD_TARGETS=$TARGETS
VPNX3_CLIENT_CONTROL_URL=$CLIENT_CONTROL_URL
VPNX3_CONFIG_PUBLIC_KEY=$CONFIG_PUBLIC_KEY
VPNX3_RELEASE_PUBLIC_KEY=$RELEASE_PUBLIC_KEY
VPNX3_IOS_TEAM_ID=$IOS_TEAM_ID
VPNX3_IOS_EXPORT_OPTIONS_PLIST=$IOS_EXPORT_OPTIONS_PLIST
VPNX3_AGENT_IDENTITY_PATH=$WORK_ROOT/identity.json
VPNX3_BUILD_WORK_ROOT=$WORK_ROOT/work
HOME=$HOME
EOF
chmod 600 "$WORK_ROOT/worker.env"

cat >"$WORK_ROOT/run.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
ENV_FILE="$HOME/Library/Application Support/VPNX3BuildWorker/worker.env"
set -a
source "$ENV_FILE"
set +a
exec /usr/local/bin/vpnx3-build-worker
EOF
chmod 700 "$WORK_ROOT/run.sh"

cat >"$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>ru.vpnx3.build-worker</string>
  <key>ProgramArguments</key>
  <array>
    <string>$WORK_ROOT/run.sh</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>$WORK_ROOT/stdout.log</string>
  <key>StandardErrorPath</key>
  <string>$WORK_ROOT/stderr.log</string>
  <key>ProcessType</key>
  <string>Background</string>
</dict>
</plist>
EOF
chmod 600 "$PLIST_PATH"

launchctl bootout "gui/$(id -u)/ru.vpnx3.build-worker" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH"
launchctl enable "gui/$(id -u)/ru.vpnx3.build-worker"
launchctl kickstart -k "gui/$(id -u)/ru.vpnx3.build-worker"

echo "VPNX3 macOS Build Worker installed."
echo "Logs: $WORK_ROOT/stdout.log and stderr.log"
echo "Xcode signing identities remain managed only in the local macOS Keychain."
