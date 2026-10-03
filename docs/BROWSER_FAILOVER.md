# Browser reconnect and failover

When browser VPN is enabled, the extension creates a one-minute maintenance alarm.

Each pass:
1. keeps the existing proxy in place;
2. refreshes Proxy Lease only when it is within 5 minutes of expiry;
3. fetches and verifies the latest signed Configuration Manifest;
4. selects ingress by priority -> independent probe latency -> health;
5. changes browser proxy only when the selected ingress actually changed.

A transient Control Plane error does not call disconnect and does not fall back to DIRECT. The current proxy remains configured until an explicit user disconnect or browser policy removes it.

Chrome/Firefox proxy error events also trigger an immediate best-effort maintenance pass.
