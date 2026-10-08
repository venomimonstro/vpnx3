# Offline trust root and signed operational keyring

VPNX3 separates the long-lived root of trust from operational signing keys.

## Trust model

The offline root private key MUST NOT be present on Control Plane, workers, ingress,
config mirrors, build workers or client devices.

Runtime infrastructure receives only:

- pinned root public key;
- a root-signed trust bundle;
- operational private keys for the services that actually sign runtime objects.

The bundle authorizes operational Ed25519 keys for three purposes:

- config;
- access;
- release.

A purpose has one active key and may include a pre-published next key. Retired keys
may remain in a bundle during a controlled transition window.

## Offline generation

Example:

```bash
go run ./cmd/trust-bundle \
  --root-seed-file /offline/root.seed \
  --version 1 \
  --valid-days 90 \
  --config-public-key "$CONFIG_PUBLIC" \
  --access-public-key "$ACCESS_PUBLIC" \
  --release-public-key "$RELEASE_PUBLIC" \
  --out trust-bundle.json
```

The resulting JSON may be copied to Control Plane/config mirrors. The root seed stays
offline.

## Rotation sequence

1. generate a new operational key offline/on the protected signing host;
2. publish it as `next` in a higher-version root-signed bundle;
3. distribute the bundle and verify client/node adoption;
4. promote the new key to `active` in another higher-version bundle;
5. switch runtime signer private key;
6. keep the previous key as `retired` only for the bounded compatibility window;
7. remove it in a later bundle.

A bundle has a monotonic version, expiry and rollback protection.
