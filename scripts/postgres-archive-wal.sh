#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_WAL_ARCHIVE_DIR:?VPNX3_WAL_ARCHIVE_DIR is required}"
: "${VPNX3_WAL_AGE_RECIPIENT:?VPNX3_WAL_AGE_RECIPIENT is required}"

SOURCE="${1:-}"
NAME="${2:-}"
[[ -f "$SOURCE" ]] || { echo "WAL source missing" >&2; exit 1; }
[[ "$VPNX3_WAL_ARCHIVE_DIR" == /* ]] || { echo "archive dir must be absolute" >&2; exit 2; }
[[ "$VPNX3_WAL_AGE_RECIPIENT" == age1* ]] || { echo "invalid age recipient" >&2; exit 2; }

case "$NAME" in
  [0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F]) ;;
  [0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F].history) ;;
  [0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F].*.backup) ;;
  *) echo "Unsafe WAL archive filename: $NAME" >&2; exit 2;;
esac

for cmd in age sha256sum; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing $cmd" >&2; exit 1; }
done

mkdir -p "$VPNX3_WAL_ARCHIVE_DIR"
chmod 700 "$VPNX3_WAL_ARCHIVE_DIR"
FINAL="$VPNX3_WAL_ARCHIVE_DIR/$NAME.age"
SIDECAR="$FINAL.sha256"
if [[ -s "$FINAL" && -s "$SIDECAR" ]]; then
  (cd "$VPNX3_WAL_ARCHIVE_DIR" && sha256sum -c "$(basename "$SIDECAR")" >/dev/null)
  exit 0
fi

TMP="$FINAL.partial.$$"
rm -f "$TMP"
trap 'rm -f "$TMP"' EXIT
age -r "$VPNX3_WAL_AGE_RECIPIENT" -o "$TMP" "$SOURCE"
chmod 600 "$TMP"
mv "$TMP" "$FINAL"
sha256sum "$FINAL" >"$SIDECAR"
chmod 600 "$SIDECAR"

# fsync directory metadata where available.
python3 - "$VPNX3_WAL_ARCHIVE_DIR" <<'PY'
import os,sys
fd=os.open(sys.argv[1],os.O_RDONLY)
try: os.fsync(fd)
finally: os.close(fd)
PY
exit 0
