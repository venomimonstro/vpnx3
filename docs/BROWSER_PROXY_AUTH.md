# Browser proxy authentication

Chrome MV3 uses webRequest + webRequestAuthProvider and asyncBlocking for proxy credentials.
Firefox MV3 keeps webRequestBlocking for proxy/system requests and also declares webRequestAuthProvider for cross-browser compatibility.

To prevent authentication loops:
1. first 407 challenge uses the cached still-valid Proxy Lease;
2. second challenge forces a fresh Proxy Lease from Control Plane;
3. third challenge cancels the request instead of repeatedly sending bad credentials.

Per-request counters are removed on completion/error.
