#!/usr/bin/env bash
# Renew OpenVPN revocation list without rotating certificates or dropping clients.
set -Eeuo pipefail
umask 077
ROOT=/etc/openvpn/vpnx3
PKI="$ROOT/pki"
CRL="$PKI/crl.pem"
[[ $EUID -eq 0 ]] || { echo "[OpenVPN] Run as root" >&2; exit 1; }
[[ -f "$PKI/ca.crt" && -f "$PKI/private/ca.key" ]] || {
  echo "[OpenVPN] Private CA unavailable; CRL cannot be renewed safely" >&2; exit 1;
}
[[ -x /usr/share/easy-rsa/easyrsa ]] || exit 1
tmp="$(mktemp -d "$ROOT/.crl-backup.XXXXXXXX")"
chmod 700 "$tmp"
if [[ -f "$CRL" ]]; then cp -p "$CRL" "$tmp/crl.pem"; fi
rollback(){
  if [[ -f "$tmp/crl.pem" ]]; then cp -p "$tmp/crl.pem" "$CRL"; fi
  rm -rf "$tmp"
}
trap rollback ERR INT TERM
(
  cd "$ROOT"
  EASYRSA_BATCH=1 EASYRSA_PKI="$PKI" EASYRSA_CRL_DAYS=180 /usr/share/easy-rsa/easyrsa gen-crl >/dev/null
)
openssl crl -in "$CRL" -noout -nextupdate >/dev/null
chmod 644 "$CRL"
trap - ERR INT TERM
rm -rf "$tmp"
echo "[OpenVPN] CRL renewed and validated; existing client certificates preserved."
