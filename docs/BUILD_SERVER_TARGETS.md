# Build Factory server targets

Build Factory now supports fixed Linux/amd64 recipes in addition to mobile/browser artifacts:

- controlplane_linux_amd64
- node_agent_linux_amd64
- vpn_worker_linux_amd64
- probe_agent_linux_amd64
- ingress_proxy_linux_amd64

All are built with CGO_ENABLED=0, GOOS=linux, GOARCH=amd64, -trimpath and stripped symbols.

Browser ZIP recipes now rewrite manifest.json version from the release version. Accepted extension version format is 1–4 numeric components; optional leading v and prerelease suffix are removed before validation. Each component must be 0..65535.

No arbitrary build command is accepted from Control Plane.
