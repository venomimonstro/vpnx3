# Ingress distributed health

Browser ingress exposes a direct unauthenticated health endpoint:

GET /__vpnx3/health

It returns only service liveness and does not expose proxy credentials, node identity or configuration.

Probe Agent now checks:
- worker HTTPS session_api;
- ingress HTTPS /__vpnx3/health.

Both result types enter the same signed probe report stream and feed the existing multi-probe health score/circuit breaker.

An active ingress that fails consistently from at least two independent probes can therefore be moved to degraded and disappear from the next signed Configuration Manifest.
