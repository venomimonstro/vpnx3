# Browser extensions

Chrome/Firefox clients now:
- generate ECDSA P-256 device identity;
- re-import private key as non-extractable CryptoKey and persist it in IndexedDB;
- register platform chrome/firefox;
- verify signed Configuration Manifest with pinned Ed25519 config key;
- select active ingress endpoint transport=http-connect;
- request short-lived signed Proxy Lease;
- configure HTTPS browser proxy;
- answer proxy auth challenges with scoped credential;
- fail closed and clear proxy when reconnect on startup fails.

Build worker overwrites runtime-config.js from VPNX3_CLIENT_CONTROL_URL and VPNX3_CONFIG_PUBLIC_KEY before ZIP creation. Source placeholder is not production-ready by design.
