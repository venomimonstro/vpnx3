# iOS release acceptance

Build Factory запускает `scripts/validate-ios-release.sh` после `xcodebuild -exportArchive` и до загрузки IPA.

Проверяется:

- codesign всего приложения;
- codesign PacketTunnel extension;
- bundle ID приложения `ru.vpnx3.app`;
- bundle ID extension `ru.vpnx3.app.PacketTunnel`;
- версия из Release;
- наличие Packet Tunnel extension;
- entitlement `packet-tunnel-provider`;
- отсутствие `get-task-allow=true` в distribution build;
- отсутствие localhost/dev URL и приватных ключей внутри пакета.

Это не заменяет установку на физический iPhone и проверку NetworkExtension entitlement в реальном provisioning profile/App Store Connect.
