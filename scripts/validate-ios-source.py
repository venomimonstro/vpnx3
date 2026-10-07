#!/usr/bin/env python3
import pathlib,re,sys

root=pathlib.Path(__file__).resolve().parents[1]
ios=root/"apps"/"ios"
errors=[]

manager=(ios/"VPNX3"/"TunnelManager.swift").read_text("utf-8")
repository=(ios/"VPNX3"/"VPNRepository.swift").read_text("utf-8")
provider=(ios/"PacketTunnel"/"PacketTunnelProvider.swift").read_text("utf-8")
shared=(ios/"Shared"/"TunnelKeyStores.swift").read_text("utf-8")

for name,text in [("TunnelManager",manager),("VPNRepository",repository)]:
    if "wgQuickConfig" in text or "PrivateKey =" in text:
        errors.append(f"{name} serializes WireGuard private configuration outside PacketTunnel")

if '"keyMode"' not in manager or '"sessionID"' not in manager:
    errors.append("TunnelManager provider configuration is missing safe session/key references")

if "PersonalKeyStore().ensure().privateKey" not in provider:
    errors.append("PacketTunnel does not load personal private key locally")
if "StandardTunnelKeyStore().ensure()" not in provider:
    errors.append("PacketTunnel does not load standard private key locally")

required=[
    "kSecAttrAccessGroup",
    "kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly",
    "kSecAttrSynchronizable",
]
for token in required:
    if token not in shared:
        errors.append(f"shared Keychain invariant missing: {token}")

for ent in [ios/"VPNX3"/"VPNX3.entitlements",ios/"PacketTunnel"/"PacketTunnel.entitlements"]:
    text=ent.read_text("utf-8")
    if "packet-tunnel-provider" not in text:
        errors.append(f"{ent.name}: packet tunnel entitlement missing")
    if "ru.vpnx3.shared" not in text:
        errors.append(f"{ent.name}: shared Keychain access group missing")

if errors:
    for e in errors: print("FAIL:",e,file=sys.stderr)
    raise SystemExit(1)

print("[OK] iOS source security invariants validated")
