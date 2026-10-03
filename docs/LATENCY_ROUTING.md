# Latency-aware routing

Configuration Manifest now carries an optional node-level latency_ms calculated only from successful independent probe observations from the last 3 minutes.

Routing order:

1. explicit administrative endpoint priority;
2. lower fresh probe latency;
3. higher health score.

If no fresh latency exists, the client treats it as unknown/worst, not as zero.

Android and Chrome/Firefox use the same ordering. The metric is produced by Control Plane from probe_results and is covered by the existing Ed25519 Configuration Manifest signature, so clients do not trust latency claimed by the VPN node itself.
