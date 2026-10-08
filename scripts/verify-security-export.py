#!/usr/bin/env python3
import argparse
import hashlib
import hmac
import json
import sys
import time

def main():
    p=argparse.ArgumentParser(description="Verify one VPNX3 external audit webhook")
    p.add_argument("--secret",required=True)
    p.add_argument("--timestamp",required=True)
    p.add_argument("--signature",required=True,help="sha256=<hex>")
    p.add_argument("--max-age",type=int,default=300)
    args=p.parse_args()

    body=sys.stdin.buffer.read()
    try:
        ts=int(args.timestamp)
    except ValueError:
        raise SystemExit("invalid timestamp")
    if abs(int(time.time())-ts)>args.max_age:
        raise SystemExit("timestamp outside allowed window")
    if not args.signature.startswith("sha256="):
        raise SystemExit("invalid signature prefix")

    expected=hmac.new(
        args.secret.encode(),
        args.timestamp.encode()+b"\n"+body,
        hashlib.sha256,
    ).hexdigest()
    if not hmac.compare_digest(expected,args.signature[7:]):
        raise SystemExit("signature mismatch")

    event=json.loads(body)
    required=["audit_id","prev_hash","entry_hash","actor_type","action","resource_type","result","created_at"]
    missing=[k for k in required if k not in event]
    if missing:
        raise SystemExit("missing fields: "+",".join(missing))
    if len(event["entry_hash"])!=64 or len(event["prev_hash"]) not in (0,64):
        raise SystemExit("invalid hash length")

    print(json.dumps({
        "status":"ok",
        "audit_id":event["audit_id"],
        "prev_hash":event["prev_hash"],
        "entry_hash":event["entry_hash"],
    },ensure_ascii=False))

if __name__=="__main__":
    main()
