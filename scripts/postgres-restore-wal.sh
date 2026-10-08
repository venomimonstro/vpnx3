#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_WAL_RESTORE_DIR:?VPNX3_WAL_RESTORE_DIR is required}"
: "${VPNX3_WAL_AGE_IDENTITY:?VPNX3_WAL_AGE_IDENTITY is required}"

NAME="${1:-}"
DEST="${2:-}"
[[ "$DEST" == /* ]] || { echo "restore destination must be absolute" >&2; exit 2; }
[[ -f "$VPNX3_WAL_AGE_IDENTITY" ]] || { echo "age identity missing" >&2; exit 1; }

case "$NAME" in
  [0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F]) ;;
  [0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F].history) ;;
  [0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F].*.backup) ;;
  *) exit 1;;
esac

ENC="$VPNX3_WAL_RESTORE_DIR/$NAME.age"
[[ -f "$ENC" ]] || exit 1
if [[ -f "$ENC.sha256" ]]; then
  (cd "$VPNX3_WAL_RESTORE_DIR" && sha256sum -c "$(basename "$ENC.sha256")" >/dev/null) || exit 1
fi

TMP="$DEST.partial.$$"
rm -f "$TMP"
trap 'rm -f "$TMP"' EXIT
age -d -i "$VPNX3_WAL_AGE_IDENTITY" -o "$TMP" "$ENC"
chmod 600 "$TMP"
mv "$TMP" "$DEST"
exit 0
