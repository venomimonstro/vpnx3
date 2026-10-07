#!/usr/bin/env python3
import argparse, json, pathlib, zipfile, tempfile, subprocess, sys

def fail(msg):
    print("FAIL:",msg,file=sys.stderr);raise SystemExit(1)

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--zip",required=True)
    ap.add_argument("--browser",choices=["chrome","firefox"],required=True)
    args=ap.parse_args()
    archive=pathlib.Path(args.zip)
    if not archive.is_file():fail("archive missing")
    with tempfile.TemporaryDirectory() as tmp:
        with zipfile.ZipFile(archive) as z:
            for member in z.infolist():
                name=pathlib.PurePosixPath(member.filename)
                if name.is_absolute() or ".." in name.parts:
                    fail("unsafe ZIP path")
            z.extractall(tmp)
        validator=pathlib.Path(__file__).with_name("validate-browser-extension.py")
        result=subprocess.run(
            [sys.executable,str(validator),"--browser",args.browser,"--dir",tmp],
            text=True,capture_output=True
        )
        if result.returncode:
            sys.stderr.write(result.stdout+result.stderr);raise SystemExit(result.returncode)
        manifest=json.loads((pathlib.Path(tmp)/"manifest.json").read_text("utf-8"))
        print(json.dumps({
            "status":"ok","browser":args.browser,"version":manifest.get("version"),
            "archive_bytes":archive.stat().st_size
        },ensure_ascii=False,indent=2))

if __name__=="__main__":main()
