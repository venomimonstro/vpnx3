# Android account and multi-device UX

Android reads authoritative account status from Control Plane and refreshes it on resume, including after return from payment confirmation.

Displayed state:
- entitlement/plan;
- expiration;
- active devices / device_limit.

Multi-device:
- paid account can generate a 10-minute pairing code while a slot is available;
- second already-installed Android app can enter the code;
- server moves its anonymous trial device into the paid account;
- local user_id is updated only after a successful signed server response.

Pairing claim is serialized by locking the target users row before device count validation, preventing concurrent claims from exceeding device_limit.
