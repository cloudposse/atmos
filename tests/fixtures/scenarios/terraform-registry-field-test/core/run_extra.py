"""Upgrade, edition, affected, failure, cancellation and PTY field experiments."""
import json
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import subprocess
import sys
import time

import yaml

from run import Case


def simple(c):
    path = c.project / "stacks/deploy/dev.yaml"
    # Keep only the independent producer for interruption and edition experiments.
    text = path.read_text()
    path.write_text(text.split("    consumer:\n")[0])
    return path


def upgrade(root, engine, baseline, candidate):
    c = Case(root / (engine + "-upgrade"), "baseline-to-candidate", engine, baseline)
    dev = simple(c)
    c.command("baseline-apply", ["terraform", "apply", "producer", "-s", "dev", "-auto-approve"], [0])
    before = c.state_snapshot()
    c.output("baseline-output")
    c.binary = candidate
    c.command("candidate-no-change", ["terraform", "plan", "producer", "-s", "dev", "-detailed-exitcode"], [0])
    c.check("candidate plan does not mutate baseline state", c.state_snapshot() == before)
    c.binary = baseline
    dev.write_text(dev.read_text().replace("dev-producer-v1", "dev-producer-v2"))
    c.command("baseline-reviewed-plan", ["terraform", "plan", "producer", "-s", "dev", "-detailed-exitcode"], [2])
    dev.write_text(dev.read_text().replace("dev-producer-v2", "dev-producer-v3"))
    c.binary = candidate
    c.command("candidate-apply-baseline-plan", ["terraform", "apply", "producer", "-s", "dev", "--from-plan"], [0])
    output = c.output("candidate-reviewed-output")
    c.check("candidate honors baseline saved input", output.get("name", {}).get("value") == "dev-producer-v2", output)
    dev.write_text(dev.read_text().replace("dev-producer-v3", "dev-producer-v2"))
    c.command("candidate-stable-plan", ["terraform", "plan", "producer", "-s", "dev", "-detailed-exitcode"], [0])
    c.save()


def affected(c):
    dev = simple(c)
    subprocess.run(["git", "add", "."], cwd=c.project, env=c.env, check=True, capture_output=True)
    subprocess.run(["git", "-c", "commit.gpgsign=false", "-c", "user.name=Field Test", "-c", "user.email=field@example.invalid", "commit", "-qm", "simple baseline"],
                   cwd=c.project, env=c.env, check=True, capture_output=True)
    base = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=c.project, text=True).strip()
    dev.write_text(dev.read_text().replace("dev-producer-v1", "dev-producer-v2"))
    subprocess.run(["git", "add", "."], cwd=c.project, env=c.env, check=True, capture_output=True)
    subprocess.run(["git", "-c", "commit.gpgsign=false", "-c", "user.name=Field Test", "-c", "user.email=field@example.invalid", "commit", "-qm", "producer change"],
                   cwd=c.project, env=c.env, check=True, capture_output=True)
    c.command("describe-affected", ["describe", "affected", "--ref=" + base, "--format=json"], [0])
    c.command("plan-affected", ["terraform", "plan", "--affected", "--ref=" + base, "-s", "dev"], [0])
    c.command("plan-affected-tags", ["terraform", "plan", "--affected", "--ref=" + base, "--tags=foundation", "-s", "dev"], [0])
    c.command("invalid-all-affected", ["terraform", "plan", "--affected", "--all", "--ref=" + base, "-s", "dev"], [1])
    c.save()


def editions(c):
    simple(c)
    c.command("apply", ["terraform", "apply", "producer", "-s", "dev", "-auto-approve"], [0])
    for date in ("2026-09-11", "2026-09-12", "2026-09-30", "2026-10-01", "2026-10-12", "2026-10-13",
                 "2026-10-18", "2026-10-19", "2026-10-20", "2026-10-25", "2026-10-26"):
        c.command("config-" + date, ["describe", "config", "-f", "json", "--edition=" + date], [0])
        c.command("plan-" + date, ["terraform", "plan", "producer", "-s", "dev", "--edition=" + date], [0])
    c.command("old-edition-explicit-auto", ["terraform", "plan", "producer", "-s", "dev", "--edition=2026-09-11", "--init-mode=auto"], [0])
    c.command("flag-over-env", ["describe", "edition", "--edition=2026-10-26"], [0], env={"ATMOS_EDITION": "2026-09-11"})
    c.command("invalid-edition", ["describe", "edition", "--edition=not-a-date"], [1])
    c.save()


def interrupted(c):
    simple(c)
    main = c.project / "components/terraform/marker/main.tf"
    main.write_text(main.read_text().replace("echo 'create ${self.input.name}'", "sleep 30; echo 'create ${self.input.name}'"))
    cmd = [c.binary, "terraform", "apply", "producer", "-s", "dev", "-auto-approve"]
    started = time.time()
    p = subprocess.Popen(cmd, cwd=c.project, env=c.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    trace = c.root / "trace.jsonl"
    while time.time() - started < 15:
        if p.poll() is not None:
            break
        if trace.exists() and any(json.loads(x)["argv"][0] == "apply" for x in trace.read_text().splitlines()):
            time.sleep(1)
            break
        time.sleep(.1)
    p.send_signal(signal.SIGINT)
    signal_time = time.time()
    forced = False
    try:
        out, err = p.communicate(timeout=10)
    except subprocess.TimeoutExpired:
        forced = True
        os.killpg(p.pid, signal.SIGKILL)
        out, err = p.communicate()
    live = []
    for row in subprocess.check_output(["ps", "ax", "-o", "pid=,pgid=,command="], text=True).splitlines():
        fields = row.strip().split(None, 2)
        if len(fields) == 3 and fields[1] == str(p.pid):
            live.append(row)
    (c.root / "interrupt.stdout").write_bytes(out)
    (c.root / "interrupt.stderr").write_bytes(err)
    (c.root / "interrupt.json").write_text(json.dumps({"command": cmd, "exit": p.returncode, "forced_cleanup": forced,
                                                     "seconds_after_signal": time.time()-signal_time, "remaining_group": live}, indent=2))
    if live:
        os.killpg(p.pid, signal.SIGKILL)
    c.check("interruption exits without forced kill", not forced)
    c.check("interruption leaves no child group", not live, live)
    c.command("plan-after-cancellation", ["terraform", "plan", "producer", "-s", "dev"], [0])
    c.save()


def terminal(c):
    simple(c)
    config_path = c.project / "atmos.yaml"
    config = yaml.safe_load(config_path.read_text())
    config["components"]["terraform"]["command"] = shutil.which(c.engine)
    config_path.write_text(yaml.safe_dump(config, sort_keys=False))
    cmd = [c.binary, "terraform", "plan", "producer", "-s", "dev", "--ui", "--force-tty", "--force-color"]
    pid, master = pty.fork()
    if pid == 0:
        os.chdir(c.project)
        os.execve(c.binary, cmd, {**c.env, "TERM": "xterm-256color"})
    chunks, start = [], time.time()
    status = None
    while time.time() - start < 30:
        ready, _, _ = select.select([master], [], [], .1)
        if ready:
            try:
                data = os.read(master, 65536)
            except OSError:
                break
            if not data:
                break
            chunks.append((time.time()-start, data))
        exited, value = os.waitpid(pid, os.WNOHANG)
        if exited:
            status = value
            break
    if status is None:
        exited, value = os.waitpid(pid, os.WNOHANG)
        if not exited:
            os.kill(pid, signal.SIGKILL)
            _, value = os.waitpid(pid, 0)
        status = value
    os.close(master)
    code = os.waitstatus_to_exitcode(status)
    (c.root / "terminal.raw").write_bytes(b"".join(data for _, data in chunks))
    (c.root / "terminal.json").write_text(json.dumps({"command": cmd, "exit": code,
                                                    "chunks": [{"seconds": t, "bytes": len(data)} for t,data in chunks]}, indent=2))
    c.check("PTY plan succeeds", code == 0)
    c.check("PTY emits progress before completion", len(chunks) > 1, len(chunks))
    c.save()


if __name__ == "__main__":
    root, baseline, candidate = sys.argv[1:]
    root = Path(root).resolve()
    baseline, candidate = str(Path(baseline).resolve()), str(Path(candidate).resolve())
    for engine in ("terraform", "tofu"):
        upgrade(root, engine, baseline, candidate)
        for revision, binary in (("baseline", baseline), ("candidate", candidate)):
            for name, operation in (("affected", affected), ("editions", editions), ("interrupt", interrupted), ("terminal", terminal)):
                operation(Case(root / f"{engine}-{revision}-{name}", revision, engine, binary))
