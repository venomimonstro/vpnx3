#!/usr/bin/env python3
import argparse
import concurrent.futures
import json
import math
import ssl
import statistics
import time
import urllib.error
import urllib.request
from collections import Counter
from urllib.parse import urlparse

MAX_CONCURRENCY = 100
MAX_DURATION = 300
MAX_REQUESTS = 20000

def percentile(values, pct):
    if not values:
        return None
    ordered=sorted(values)
    pos=(len(ordered)-1)*(pct/100.0)
    lo=math.floor(pos); hi=math.ceil(pos)
    if lo==hi:
        return ordered[lo]
    return ordered[lo]*(hi-pos)+ordered[hi]*(pos-lo)

def fetch(url, timeout):
    start=time.perf_counter()
    status=0
    error=""
    try:
        req=urllib.request.Request(url,headers={"Accept":"application/json","User-Agent":"vpnx3-load-smoke/1"})
        with urllib.request.urlopen(req,timeout=timeout,context=ssl.create_default_context()) as resp:
            status=resp.status
            resp.read(4096)
    except urllib.error.HTTPError as exc:
        status=exc.code
        error=f"http_{exc.code}"
    except Exception as exc:
        error=type(exc).__name__
    latency=(time.perf_counter()-start)*1000.0
    return status,error,latency

def main():
    ap=argparse.ArgumentParser(description="Bounded VPNX3 HTTP load smoke; no CI required.")
    ap.add_argument("--url",action="append",required=True,help="HTTPS URL to test; repeatable")
    ap.add_argument("--concurrency",type=int,default=10)
    ap.add_argument("--duration",type=int,default=30,help="seconds")
    ap.add_argument("--timeout",type=float,default=5.0)
    ap.add_argument("--max-requests",type=int,default=5000)
    ap.add_argument("--fail-error-rate",type=float,default=1.0,help="percent")
    ap.add_argument("--fail-p95-ms",type=float,default=1000.0)
    args=ap.parse_args()

    if not (1 <= args.concurrency <= MAX_CONCURRENCY):
        raise SystemExit(f"concurrency must be 1..{MAX_CONCURRENCY}")
    if not (1 <= args.duration <= MAX_DURATION):
        raise SystemExit(f"duration must be 1..{MAX_DURATION} seconds")
    if not (1 <= args.max_requests <= MAX_REQUESTS):
        raise SystemExit(f"max-requests must be 1..{MAX_REQUESTS}")
    if not (0.2 <= args.timeout <= 30):
        raise SystemExit("timeout must be 0.2..30 seconds")
    if not (0 <= args.fail_error_rate <= 100):
        raise SystemExit("fail-error-rate must be 0..100")
    if args.fail_p95_ms <= 0:
        raise SystemExit("fail-p95-ms must be positive")

    urls=[]
    for raw in args.url:
        parsed=urlparse(raw)
        if parsed.scheme!="https" or not parsed.hostname:
            raise SystemExit(f"only explicit https URLs are allowed: {raw}")
        if parsed.username or parsed.password:
            raise SystemExit("credentials in URLs are forbidden")
        urls.append(raw)

    deadline=time.monotonic()+args.duration
    launched=0
    results=[]
    started=time.monotonic()

    def task(i):
        return urls[i % len(urls)], fetch(urls[i % len(urls)],args.timeout)

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        inflight=set()
        while time.monotonic()<deadline and launched<args.max_requests:
            while len(inflight)<args.concurrency and launched<args.max_requests and time.monotonic()<deadline:
                fut=pool.submit(task,launched)
                inflight.add(fut)
                launched+=1
            if not inflight:
                break
            done,inflight=concurrent.futures.wait(
                inflight,timeout=0.2,return_when=concurrent.futures.FIRST_COMPLETED
            )
            for fut in done:
                results.append(fut.result())
        for fut in concurrent.futures.as_completed(inflight):
            results.append(fut.result())

    elapsed=max(0.001,time.monotonic()-started)
    by_url={}
    for url,(status,error,latency) in results:
        bucket=by_url.setdefault(url,{"latencies":[],"statuses":Counter(),"errors":Counter()})
        bucket["latencies"].append(latency)
        bucket["statuses"][status]+=1
        if error:
            bucket["errors"][error]+=1

    total=len(results)
    total_errors=sum(1 for _,(status,error,_) in results if error or not (200 <= status < 300))
    error_rate=(total_errors/total*100.0) if total else 100.0
    all_lat=[lat for _,(_,_,lat) in results]
    p95=percentile(all_lat,95) or 0.0

    report={
        "requests":total,
        "elapsed_seconds":round(elapsed,3),
        "requests_per_second":round(total/elapsed,2),
        "error_rate_percent":round(error_rate,3),
        "latency_ms":{
            "mean":round(statistics.fmean(all_lat),2) if all_lat else None,
            "p50":round(percentile(all_lat,50),2) if all_lat else None,
            "p95":round(p95,2) if all_lat else None,
            "p99":round(percentile(all_lat,99),2) if all_lat else None,
            "max":round(max(all_lat),2) if all_lat else None,
        },
        "targets":{}
    }
    for url,bucket in by_url.items():
        lat=bucket["latencies"]
        report["targets"][url]={
            "requests":len(lat),
            "statuses":dict(bucket["statuses"]),
            "errors":dict(bucket["errors"]),
            "p95_ms":round(percentile(lat,95),2) if lat else None,
        }

    print(json.dumps(report,ensure_ascii=False,indent=2))

    failed=False
    if error_rate > args.fail_error_rate:
        print(f"FAIL: error rate {error_rate:.3f}% > {args.fail_error_rate:.3f}%",file=__import__("sys").stderr)
        failed=True
    if p95 > args.fail_p95_ms:
        print(f"FAIL: p95 {p95:.2f} ms > {args.fail_p95_ms:.2f} ms",file=__import__("sys").stderr)
        failed=True
    raise SystemExit(1 if failed else 0)

if __name__=="__main__":
    main()
