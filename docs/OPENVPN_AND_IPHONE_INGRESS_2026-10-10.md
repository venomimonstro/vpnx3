# VPNX3: Why local OpenVPN and iPhone checks can disagree

**Evidence from actual operator VPS logs (2026-10-10):**

- VLESS Vision + iOS Xray test clients reached `example.com`,
  `telegram.org` and `web.telegram.org` through the VPN tunnel from the VPS.
- Runtime audit returned `FAIL=0 WARN=0`: host service, certs, NAT, and Docker
  checks are consistent at a point in time.
- The separate OpenVPN client namespace exited before installing the VPN
  default route. The old diagnostic only printed a generic exit message.
- These tests were initiated ON THE VPS. None verifies RF ISP ingress or
  native iPhone Telegram traffic.

## Confirmed weakness in local OpenVPN acceptance

The old client test started from an isolated network namespace on the VPS
using a real private health-check profile whose `remote` was the **public**
IP of the VPS. This can rely on public-IP hairpin routing through the provider
network: such traffic may never return to the host, regardless of whether
OpenVPN service itself works.

The corrected acceptance creates a temporary root-only profile that changes
only the `remote` address to the host-side veth gateway
(`10.254.242.1`). It connects to the REAL OpenVPN daemon on local UDP/1194,
negotiates TLS with the same CA/health client certificate, and checks routes
and Telegram HTTPS through the resulting tunnel. It leaves the primary VPS
routing table and the iPhone's profile untouched; temp profile is deleted
after the test.

If the isolated client still fails, the diagnostic prints a sanitized failure
category (TUN access, invalid profile, TLS, routing, namespace). Exact server
logs and out-of-band testing may still be needed.

## EXTERNAL iPhone ingress check

Use this while reconnecting **from a different network**, ideally iPhone
mobile data (Wi-Fi off):

```bash
sudo python3 scripts/diagnose-vpn-arrivals.py
```

The script listens on the PUBLIC network interface for TCP SYN packets
to actual published VLESS ports (e.g. 8443, 2053) and UDP to the configured
OpenVPN port (usually 1194). It outputs counts ONLY, no source IPs or payload.

- `0` packets during confirmed reconnection attempts suggests an upstream
  issue (wrong imported endpoint/port, device routing, ISP filtering, provider
  firewall). It is NOT an OpenVPN CA/REALITY key mismatch.
- Incoming VLESS TCP SYN without a successful client connection shifts
  debugging to handshake, client mode, SNI/shortId, firewall response and
  routing. A SYN does NOT prove TLS was established.
- Incoming UDP to OpenVPN without a tunnel shifts debugging to certificate,
  `tls-crypt`, data-cipher negotiation and NAT. UDP arrival does NOT prove
  a VPN handshake.
- Zero traffic is inconclusive if the phone did not try to connect during the
  45-second capture.
- End-to-end working Telegram iOS requires confirming app traffic routes through
  the selected tunnel, DNS, IPv4/IPv6, and stable mobile reconnect. Successful
  `telegram.org` HTTPS on the VPS is not equivalent.

**Do not change keys, REALITY SNI, or admin port 443 during diagnostics.**

Refs: OpenVPN 2.6 manual
https://openvpn.net/community-docs/community-articles/openvpn-2-6-manual.html
and official INCY routing and import documentation https://docs.incy.cc/deep-links/
