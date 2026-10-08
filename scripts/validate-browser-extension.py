#!/usr/bin/env python3
import argparse
import json
import pathlib
import re
import sys

FORBIDDEN_PATTERNS = [
    re.compile(r"https?://(?:127\.0\.0\.1|localhost)(?::\d+)?", re.I),
    re.compile(r"BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY"),
    re.compile(r"VPNX3_[A-Z0-9_]*(?:SECRET|PASSWORD|TOKEN)\s*="),
]

ALLOWED_PERMISSIONS = {
    "chrome": {"proxy","storage","webRequest","webRequestAuthProvider","alarms"},
    "firefox": {"proxy","storage","webRequest","webRequestBlocking","webRequestAuthProvider","alarms"},
}

def fail(msg):
    print("FAIL:", msg, file=sys.stderr)
    raise SystemExit(1)

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--browser",choices=["chrome","firefox"],required=True)
    ap.add_argument("--dir",required=True)
    args=ap.parse_args()

    root=pathlib.Path(args.dir).resolve()
    if not root.is_dir():
        fail("extension directory not found")

    manifest_path=root/"manifest.json"
    if not manifest_path.is_file():
        fail("manifest.json missing")

    try:
        manifest=json.loads(manifest_path.read_text("utf-8"))
    except Exception as exc:
        fail(f"invalid manifest.json: {exc}")

    if manifest.get("manifest_version") != 3:
        fail("Manifest V3 is required")
    version=str(manifest.get("version","")).strip()
    if not re.fullmatch(r"\d+(?:\.\d+){0,3}",version):
        fail("invalid extension version")

    permissions=set(manifest.get("permissions") or [])
    unknown=permissions-ALLOWED_PERMISSIONS[args.browser]
    if unknown:
        fail("unexpected permissions: "+", ".join(sorted(unknown)))

    if args.browser=="chrome" and "webRequestAuthProvider" not in permissions:
        fail("Chrome proxy auth requires webRequestAuthProvider")
    if args.browser=="firefox" and "webRequestBlocking" not in permissions:
        fail("Firefox proxy auth requires webRequestBlocking")

    runtime=root/"runtime-config.js"
    if not runtime.is_file():
        fail("runtime-config.js missing")
    runtime_text=runtime.read_text("utf-8",errors="replace")
    if "VPNX3_CONTROL_URL" not in runtime_text or "https://" not in runtime_text:
        fail("runtime config must contain HTTPS Control URL")
    if "VPNX3_CONFIG_PUBLIC_KEY" not in runtime_text:
        fail("runtime config must contain pinned config public key")
    if "VPNX3_RELEASE_PUBLIC_KEY" not in runtime_text:
        fail("runtime config must contain pinned release public key")

    checked=0
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        if path.stat().st_size > 2*1024*1024:
            continue
        text=path.read_text("utf-8",errors="ignore")
        checked+=1
        for pattern in FORBIDDEN_PATTERNS:
            if pattern.search(text):
                fail(f"forbidden development/secret material in {path.relative_to(root)}")

    required=["background.js","popup.html","popup.js"]
    for name in required:
        if not (root/name).is_file():
            fail(f"required file missing: {name}")

    print(json.dumps({
        "status":"ok",
        "browser":args.browser,
        "version":version,
        "permissions":sorted(permissions),
        "files_checked":checked,
    },ensure_ascii=False,indent=2))

if __name__=="__main__":
    main()
