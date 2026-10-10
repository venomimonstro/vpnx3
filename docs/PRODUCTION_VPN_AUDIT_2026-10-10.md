# VPNX3: production VPN connectivity and security audit
Date: 2026-10-10. Scope: repository main and operator-provided VPS logs, not direct SSH access to VPS or actual iPhone / Russian network. No CI or GitHub Actions are used.

## First principles

* VPN service health has four independent layers: public reachability (SYN/UDP arrival), authenticated handshake, routing/DNS/NAT, actual application traffic.
* A `docker ps`, OpenVPN `systemctl active`, Xray `run -test`, or loopback VPN test proves only a subset. None proves that an RF ISP can reach a VPS or that Telegram iOS sends its traffic down the tunnel.
* Operator previously measured ZERO incoming TCP SYNs on 8443 during two 30s experiments. If a client attempted connectivity, the problem exists **before** VLESS authorization: client import/routing, network filtering, ISP/provider firewall, or public IP/port mismatch.

## P0 / P1 confirmed repo defects and fixes in main

| Area | Finding | Action |
| --- | --- | --- |
| Shared 443 | Old script assumed changing a Caddyfile frees Docker's host port 443, and treated container localhost as host localhost. Both are false in common Docker bridge layouts; can break admin HTTPS. | Unsafe enable path is deliberately blocked (fail closed), status/recovery retained. Do not claim VPN is working on 443 until a topology-aware migration has tests for both TLS and REALITY. |
| Xray secrets | `install-personal-vless.sh status` printed the full VLESS import link in shell logs. | Removed secret output in status. Links remain visible in authenticated admin only. |
| Xray endpoint | Installer's `VPNX3_PERSONAL_VLESS_IP` did not reach independently launched manager service. Wrong public endpoint could be encoded into newly created links. | Public endpoint persisted under root-only private state, manager reads it; repeat installs preserve custom endpoint unless explicitly changed. |
| OpenVPN routing | Source-subnet-only ACCEPT in FORWARD allowed spoofable broad packets and was sensitive to Docker forwarding policy; server VPN interface had no fixed name. | Dedicated tunnel interface, WAN-scoped forwarding rules, DOCKER-USER support, legacy broad-rule cleanup, correct NAT checks. |
| OpenVPN installer | Updating a systemd oneshot `RemainAfterExit=yes` service with `enable --now` does not rerun its ExecStart; new firewall rules were not installed. | Explicit restart and post-apply rule checks. |
| OpenVPN lifecycle | A partially written profile could be downloaded after a failed server-side command. | Atomic temp-file export followed by validation and rename. |
| OpenVPN revocation | CRLs have a finite `nextUpdate`; expiry can reject authenticating devices, while systemd still reports the daemon as active. | Weekly systemd renewal with backup/rollback and certificate/CRL health shown in admin. |
| Admin status | OpenVPN readiness used only `systemctl is-active`; port was reported as hardcoded 1194 regardless of server configuration. | Actual configured port, UDP listener and cert/CRL validity now reported. |
| Test quality | An unavailable OpenVPN client was `SKIP` but test exit code could still report success; completed RF MTR was counted as VPN success. | Exit 2 for incomplete client acceptance; external TCP traceroute is explicitly inconclusive about the VPN handshake. |
| Tests/diagnostics | No single runtime inspection of port ownership, Docker, OpenVPN PKI, profile drift and NAT. | Added read-only `scripts/audit-vpn-runtime.py` and manual safety regressions. |

## 2026-10-10 runtime evidence and additional P0 fix

Operator logs: VLESS in both Vision and iOS modes reached HTTPS
`example.com`, `telegram.org`, and `web.telegram.org` through a real
local Xray client. OpenVPN's daemon was active and UDP/1194 bound, but the
isolated OpenVPN client failed to establish routes.

**Confirmed critical configuration error:** `dev vpnx3tun0` is an arbitrary
OpenVPN device name (does not begin with tun/tap). The OpenVPN 2.6 manual
requires the additional `dev-type tun` directive for this. Without that
directive a newly generated config can fail on daemon start. The installer
now includes that directive, safeguards the prior config on restart failure,
and has a regression test for it.

The observed runtime config was *old* (`dev tun`, missing scoped rules
and CRL timer). Running only the control-plane deployment does not update
the host-level OpenVPN service. The operator must run
`sudo bash scripts/install-personal-openvpn.sh install` after pulling main.

The prior acceptance test reused the first user's `.ovpn` key; this can
displace a live client with the same certificate CN. It now requires
a separate root-only `vpnx3-health` certificate provisioned by the
installer and kept out of the admin device list.

OpenVPN test logs are now captured in an isolated temporary file, summarized
by error category without emitting server IPs or certificate contents.
When a test fails, inspect its named failure first instead of rotating keys.

The optional Globalping probe returned HTTP 403: **this is an unavailable
external probing service, not evidence that the VPN or Russia is blocked.**

Reference: [OpenVPN manual: --dev and --dev-type](https://openvpn.net/community-docs/community-articles/openvpn-2-6-manual.html).

## Still not proven or intentionally not changed

1. **RF mobile network**. No live authenticated VLESS/OpenVPN client is under our control in a Russian access network. Globalping probes can test some upstream connectivity, not iOS/TG app VPN usability. Need user-owned or explicitly authorized real RF device/probe in at least two independent networks.
2. **Public IP reachability and reputation.** Provider-level firewall / traffic filtering are not configurable from the GitHub repo. If IP itself is filtered, rotating keys and moving ports on the same IP won't fix it.
3. **TCP/443 consolidation.** The previous 443 architecture is unsafe. A reliable design requires declarative ownership of 443 (managed Docker Compose/host service), explicit host-vs-container listener addresses, staged migration with rollback, and acceptance tests preserving admin HTTPS; not an untested Caddyfile overwrite.
4. **Single point of failure.** One VPS (1 vCPU/1 GB RAM) and one provider have no failover. Production VPN needs at least two independent provider IPs/regions and client selection with health-based failover.
5. **IPv6 leakage and DNS policies.** The OpenVPN server currently routes IPv4 full tunnel (`redirect-gateway def1`) but does not explicitly enforce client IPv6 containment. The OpenVPN 2.6 manual documents `block-ipv6` with matching IPv6 routes. Do not claim an iPhone full-tunnel privacy guarantee until IPv6/dual-stack clients are explicitly tested. Changing IPv6 routing without testing can break clients.
6. **Protocol filtering**. OpenVPN 1194/UDP and VLESS 8443/TCP can be filtered independently. Additional protocol/port combinations require separate real network tests. No one protocol is 100% guaranteed through every RF ISP.
7. **Admin HTTPS**. Existing local-CA/self-signed certificate is not ideal for public iOS admin/download UX; production requires a hostname, trusted TLS, locked-down admin permissions, and an out-of-band recovery route.
8. **PKI/secret lifecycle**. CA private key lives on VPS for web issuance; for a large commercial service, isolate/rotate its authority or use an intermediate issuance hierarchy and offsite encrypted backups. No independent restore drill for VLESS keys and OpenVPN PKI was performed.
9. **Health telemetry**. The project lacks confirmed real-time per-provider connection success/error rates from RF devices; daemon health is not sufficient. Add opt-in privacy-conscious aggregate metrics for connect/handshake time, tunnel traffic, DNS, failure reason, reconnect frequency, and certificate expiry.
10. **Telegram iOS**. HTTPS success to `telegram.org` or `web.telegram.org` does not prove that the Telegram native app can reach its MTProto data centers with the same routing policy.

## Operator commands

Install patched services (brief OpenVPN/VLESS service restart; will not rotate keys):

```bash
cd /opt/vpnx3
git pull --ff-only origin main
sudo bash scripts/install-personal-openvpn.sh install
sudo bash scripts/install-personal-vless.sh install
sudo bash scripts/install-control-production.sh deploy
```

Runtime audit only (no changes):

```bash
sudo python3 scripts/audit-vpn-runtime.py
```

Full local acceptance + optional RF network probe:

```bash
sudo bash scripts/vpn-full-check.sh
```

The final acceptance emits `PASS`, `FAIL`, `INCOMPLETE` and `UNAVAILABLE` distinctly. Treat `INCOMPLETE` and `UNAVAILABLE` as unknown, never as successful remote VPN access.

## Launch checklist / definition of done

* Local port and container ownership verified without service port collisions.
* Operator login and admin download do not expose keys, profile issuance/revocation works.
* Both profiles complete authenticated handshake through actual clients, not just configuration validation.
* HTTP/TLS to Telegram website through **both** VLESS and OpenVPN tunnels succeeds; check tunnel-assigned exit IP.
* From an RF mobile carrier and an independent RF fixed ISP: authenticated connection, route+DNS, external IP change, HTTPS site access, Telegram iOS app message send/receive, reboot, reconnect, and leak checks.
* No critical packet loss or reconnect loop during a 24-hour user test.
* A second independent ingress/provider supports failover before marking service ready for paying users.

## Official technical references

* [Xray VLESS+REALITY server examples](https://github.com/XTLS/Xray-examples/tree/main/VLESS-TCP-XTLS-Vision-REALITY)
* [OpenVPN 2.6 community manual](https://openvpn.net/community-docs/community-articles/openvpn-2-6-manual.html)
* [OpenVPN: revoking certificates and CRL reload](https://openvpn.net/community-docs/revoking-certificates.html)
* [Docker bridge firewall/DOCKER-USER](https://docs.docker.com/engine/network/firewall-iptables/)
* [Docker port publishing and NAT](https://docs.docker.com/engine/network/port-publishing/)
* [INCY developer documentation: deep links and routing](https://docs.incy.cc/deep-links/)
* [OpenVPN Connect iOS profile import](https://openvpn.net/connect-docs/ios-installation-guide.html)
* [Globalping API: MTR from chosen locations](https://blog.globalping.io/run-mtr-with-http-using-globalping-api/)
