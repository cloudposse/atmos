"""Interrupt retry backoff with exact command/environment evidence; no cloud calls."""
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time

import yaml

from run import Case
from run_extra import simple


def retry(c):
    dev = simple(c)
    doc = yaml.safe_load(dev.read_text())
    doc["components"]["terraform"]["producer"]["retry"] = {
        "max_attempts": 3, "initial_delay": "30s", "max_delay": "30s",
        "random_jitter": 0, "conditions": ["FIELD_RETRY"]}
    dev.write_text(yaml.safe_dump(doc, sort_keys=False))
    wrapper = c.root / c.engine
    wrapper.write_text("#!" + sys.executable + "\nimport os,sys,json,time\n"
                       "with open(os.environ['TRACE_LOG'],'a') as f: f.write(json.dumps({'argv':sys.argv[1:],'cwd':os.getcwd(),'pid':os.getpid(),'time':time.time()})+'\\n')\n"
                       "if sys.argv[1]=='plan':\n print('FIELD_RETRY',flush=True)\n sys.exit(1)\n"
                       f"os.execv({shutil.which(c.engine)!r},[{shutil.which(c.engine)!r}]+sys.argv[1:])\n")
    wrapper.chmod(0o755)
    command = [c.binary, "terraform", "plan", "producer", "-s", "dev"]
    proc = subprocess.Popen(command, cwd=c.project, env=c.env, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, start_new_session=True)
    start = time.time()
    trace = c.root / "trace.jsonl"
    while proc.poll() is None and time.time() - start < 15:
        if trace.exists() and any(json.loads(line)["argv"][0] == "plan" for line in trace.read_text().splitlines()):
            time.sleep(1)
            break
        time.sleep(.05)
    signal_at = time.time()
    proc.send_signal(signal.SIGINT)
    forced = False
    try:
        out, err = proc.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        forced = True
        os.killpg(proc.pid, signal.SIGKILL)
        out, err = proc.communicate()
    calls = [json.loads(line) for line in trace.read_text().splitlines()]
    attempts = sum(call["argv"][0] == "plan" for call in calls)
    (c.root / "retry.stdout").write_bytes(out)
    (c.root / "retry.stderr").write_bytes(err)
    record = {"command": command, "cwd": str(c.project), "environment_overrides": c.env,
              "exit": proc.returncode, "seconds_after_signal": time.time()-signal_at,
              "forced_cleanup": forced, "plan_attempts": attempts, "engine_calls": calls}
    (c.root / "retry.json").write_text(json.dumps(record, indent=2) + "\n")
    c.check("SIGINT interrupts backoff without another attempt", proc.returncode == 130 and not forced and attempts == 1)
    c.save()
    print(c.engine, c.revision, record["exit"], "attempts", attempts, flush=True)


if __name__ == "__main__":
    root, baseline, candidate = sys.argv[1:]
    for engine in ("terraform", "tofu"):
        for revision, binary in (("baseline", baseline), ("candidate", candidate)):
            retry(Case(Path(root) / f"{engine}-{revision}", revision, engine, binary))
