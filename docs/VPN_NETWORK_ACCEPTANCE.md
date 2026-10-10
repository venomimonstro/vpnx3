# VPNX3 production network acceptance (2026-10)

## What failed before

A successful `xray run -test` verifies JSON configuration, not internet reachability.
A localhost SOCKS-to-Xray check verifies the VPN server's egress from the VPS,
not whether an iPhone on a Russian network can connect to the public IP.
A systemd `active` status for OpenVPN verifies a daemon, not the iOS client,
NAT, DNS, routing or access to Telegram.

Two earlier checks captured **zero SYN arrivals on 8443/TCP** while an
iPhone was supposedly connecting. That observation points upstream of
VLESS authentication, if the iPhone actually made those attempts.

## Implemented verification

`sudo bash scripts/vpn-full-check.sh` performs:

1. Shell/Python syntax checks and deterministic client profile unit tests.
2. A real Xray VLESS+REALITY SOCKS client, HTTPS/TLS to `example.com`,
   `telegram.org`, and `web.telegram.org`. It tests supported client modes.
3. If an OpenVPN client profile exists, an OpenVPN process inside a disposable
   Linux network namespace. The namespace has its own default route and
   resolv.conf, preserving the VPS production routes; test connections to
   the same official domains must traverse the OpenVPN tunnel.
4. Optionally, an independent Globalping TCP MTR measurement requested from
   a Russian probe (if one is available). This verifies a network path only.
   It **cannot** prove the VLESS handshake or Telegram through the VPN in RF.

**No arbitrary private VPN credentials are uploaded to Globalping.**
An end-to-end RF validation needs a real RF client or an explicitly authorized
RF node running the VPN client and fetching HTTPS/Telegram through it.

Telegram's iOS app uses more than web.telegram.org. Website HTTPS success is
necessary diagnostic evidence, not a guarantee that every Telegram transport
works.

## Standard TCP 443 for VLESS + REALITY

On the current one-IP VPS the admin is already listening on TCP/443.
Use a reversible SNI passthrough gateway to share this port:

```bash
sudo bash scripts/enable-vless-443.sh enable
```

- REALITY ClientHello with configured `serverName` routes to Xray on 8443.
- Other TLS (including IP-only HTTPS admin) routes to the Caddy backend
  bound to loopback TCP 9443.
- No VPN private keys, UUIDs or REALITY secrets are rotated.
- The enable command validates both admin HTTPS and a real local VLESS
  handshake over TCP 443. On error it restores the previous Caddy setup.
- After enabling, refresh the VPNX3 admin to copy the 443 URL; do not reuse
  the old 8443 URL when comparing ports.

Rollback:

```bash
sudo bash scripts/enable-vless-443.sh disable
```

Do not manually repoint or forward public TLS endpoints to the administrator
without accounting for the SNI gateway. Do not treat the localhost validation
as proof of reachability from a specific mobile provider.

## Recommended operational decisions

- Keep a dedicated administrative endpoint with limited access and a trusted
  certificate when possible. Self-signed `tls internal` requires local trust
  management and is not a permanent public-panel solution.
- Avoid repeated random changes of UUID, `shortId` and key pairs. Use
  packet arrival, tunnel handshake, route and HTTP checks in sequence.
- Record the client application, version, Wi-Fi vs cellular result and exact
  route mode before concluding a server is broken.
- When a VPS public IP itself is filtered, port hopping on that IP does not
  solve reachability. Plan independent edge nodes and external monitoring.
