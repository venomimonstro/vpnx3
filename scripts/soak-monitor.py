#!/usr/bin/env python3
import argparse
import json
import math
import ssl
import statistics
import sys
import time
import urllib.error
import urllib.request
from collections import Counter
from urllib.parse import urlparse

MAX_DURATION = 86400
MIN_INTERVAL = 5
MAX_TARGETS = 20

def percentile(values,pct):
    if not values:return None
    vals=sorted(values)
    pos=(len(vals)-1)*(pct/100.0)
    lo=math.floor(pos);hi=math.ceil(pos)
    if lo==hi:return vals[lo]
    return vals[lo]*(hi-pos)+vals[hi]*(pos-lo)

def fetch(url,timeout):
    started=time.perf_counter()
    try:
        req=urllib.request.Request(url,headers={"Accept":"application/json","User-Agent":"vpnx3-soak/1"})
        with urllib.request.urlopen(req,timeout=timeout,context=ssl.create_default_context()) as resp:
            resp.read(4096)
            return resp.status,"",(time.perf_counter()-started)*1000
    except urllib.error.HTTPError as exc:
        return exc.code,f"http_{exc.code}",(time.perf_counter()-started)*1000
    except Exception as exc:
        return 0,type(exc).__name__,(time.perf_counter()-started)*1000

def main():
    ap=argparse.ArgumentParser(description="Read-only VPNX3 soak monitor")
    ap.add_argument("--url",action="append",required=True)
    ap.add_argument("--duration",type=int,default=3600)
    ap.add_argument("--interval",type=int,default=15)
    ap.add_argument("--timeout",type=float,default=5)
    ap.add_argument("--output",default="")
    ap.add_argument("--fail-availability",type=float,default=99.0)
    ap.add_argument("--fail-p95-ms",type=float,default=1500)
    args=ap.parse_args()

    if not 60 <= args.duration <= MAX_DURATION:
        raise SystemExit(f"duration must be 60..{MAX_DURATION}")
    if args.interval < MIN_INTERVAL:
        raise SystemExit(f"interval must be >= {MIN_INTERVAL} seconds")
    if not 0.2 <= args.timeout <= 30:
        raise SystemExit("timeout must be 0.2..30")
    if not 1 <= len(args.url) <= MAX_TARGETS:
        raise SystemExit(f"1..{MAX_TARGETS} targets required")

    targets=[]
    for raw in args.url:
        p=urlparse(raw)
        if p.scheme!="https" or not p.hostname or p.username or p.password:
            raise SystemExit(f"only credential-free explicit https URLs are allowed: {raw}")
        targets.append(raw)

    sink=open(args.output,"a",encoding="utf-8") if args.output else None
    stats={u:{"lat":[],"ok":0,"total":0,"errors":Counter(),"max_streak":0,"streak":0} for u in targets}
    started=time.monotonic()
    deadline=started+args.duration

    try:
        while time.monotonic()<deadline:
            cycle=time.time()
            for url in targets:
                status,error,lat=fetch(url,args.timeout)
                ok=200 <= status < 300 and not error
                s=stats[url]
                s["total"]+=1;s["lat"].append(lat)
                if ok:
                    s["ok"]+=1;s["streak"]=0
                else:
                    s["errors"][error or f"http_{status}"]+=1
                    s["streak"]+=1
                    s["max_streak"]=max(s["max_streak"],s["streak"])
                row={
                    "ts":time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime()),
                    "url":url,"status":status,"ok":ok,"latency_ms":round(lat,2),
                    "error":error or None
                }
                line=json.dumps(row,ensure_ascii=False)
                print(line,flush=True)
                if sink:
                    sink.write(line+"\n");sink.flush()
            elapsed=time.time()-cycle
            sleep=max(0,args.interval-elapsed)
            time.sleep(sleep)
    except KeyboardInterrupt:
        pass
    finally:
        if sink:sink.close()

    report={"duration_seconds":round(time.monotonic()-started,2),"targets":{}}
    failed=False
    for url,s in stats.items():
        availability=(s["ok"]/s["total"]*100) if s["total"] else 0
        p95=percentile(s["lat"],95) or 0
        report["targets"][url]={
            "samples":s["total"],
            "availability_percent":round(availability,4),
            "latency_ms":{
                "mean":round(statistics.fmean(s["lat"]),2) if s["lat"] else None,
                "p50":round(percentile(s["lat"],50),2) if s["lat"] else None,
                "p95":round(p95,2) if s["lat"] else None,
                "p99":round(percentile(s["lat"],99),2) if s["lat"] else None,
                "max":round(max(s["lat"]),2) if s["lat"] else None,
            },
            "max_consecutive_failures":s["max_streak"],
            "errors":dict(s["errors"]),
        }
        if availability < args.fail_availability or p95 > args.fail_p95_ms:
            failed=True

    print(json.dumps(report,ensure_ascii=False,indent=2))
    raise SystemExit(1 if failed else 0)

if __name__=="__main__":
    main()
