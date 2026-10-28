"""Paired real-CLI field harness. Requires Python 3, PyYAML, Git and both engines.

All runtime changes are confined to a new output directory; the fixture is copied.
Usage: python3 run.py RUN_DIRECTORY BASELINE_BINARY CANDIDATE_BINARY
"""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time

import yaml


class Case:
    def __init__(self, root, revision, engine, binary):
        self.root = Path(root).resolve()
        self.revision, self.engine, self.binary = revision, engine, str(Path(binary).resolve())
        self.project = self.root / "project"
        self.root.mkdir(parents=True, exist_ok=False)
        shutil.copytree(Path(__file__).parent, self.project, ignore=shutil.ignore_patterns("run.py", "__pycache__"))
        self.logs = self.root / "commands"
        self.logs.mkdir()
        self.records, self.checks = [], []
        self.env = {k: os.environ[k] for k in ("PATH", "HOME", "TMPDIR", "LANG") if k in os.environ}
        self.env.update({"ATMOS_TELEMETRY_ENABLED": "false", "ATMOS_PRO_ERRORS_ENABLED": "false",
                         "ATMOS_CI": "false", "CI": "false", "CHECKPOINT_DISABLE": "1",
                         "TF_INPUT": "0", "AWS_EC2_METADATA_DISABLED": "true", "NO_COLOR": "1",
                         "XDG_CONFIG_HOME": str(self.root / "config"), "XDG_CACHE_HOME": str(self.root / "cache"),
                         "ATMOS_XDG_CACHE_HOME": str(self.root / "cache"),
                         "ORDER_LOG": str(self.root / "order.log"), "HOOK_LOG": str(self.root / "hooks.log"),
                         "TRACE_LOG": str(self.root / "trace.jsonl")})
        rc = self.root / "terraform.rc"
        rc.write_text("")
        self.env.update(TF_CLI_CONFIG_FILE=str(rc), TOFU_CLI_CONFIG_FILE=str(rc))
        wrapper = self.root / engine
        wrapper.write_text("#!" + sys.executable + "\nimport os,sys,json,time\n"
                           "with open(os.environ['TRACE_LOG'],'a') as f: f.write(json.dumps({'argv':sys.argv[1:],'cwd':os.getcwd(),'pid':os.getpid(),'time':time.time()})+'\\n')\n"
                           f"os.execv({shutil.which(engine)!r},[{shutil.which(engine)!r}]+sys.argv[1:])\n")
        wrapper.chmod(0o755)
        config = yaml.safe_load((self.project / "atmos.yaml").read_text())
        config["components"]["terraform"]["command"] = str(wrapper)
        (self.project / "atmos.yaml").write_text(yaml.safe_dump(config, sort_keys=False))
        hooks = {}
        for verb in ("init", "plan", "apply", "deploy", "destroy", "output", "refresh", "test"):
            for when in ("before", "after"):
                key = f"{when}-{verb}"
                hooks[key] = {"events": [f"{when}.terraform.{verb}"], "kind": "step", "type": "shell",
                              "with": {"command": f'echo "{key} $ATMOS_STACK/$ATMOS_COMPONENT status=$ATMOS_HOOK_STATUS" >> "$HOOK_LOG"'}}
        # Root hooks are inherited by both stacks.
        for path in (self.project / "stacks/deploy").glob("*.yaml"):
            path.write_text(yaml.safe_dump({"hooks": hooks}, sort_keys=False) + path.read_text())
        for args in (["init", "-q"], ["add", "."], ["-c", "commit.gpgsign=false", "-c", "user.name=Field Test", "-c", "user.email=field@example.invalid", "commit", "-qm", "fixture"]):
            subprocess.run(["git", *args], cwd=self.project, env=self.env, check=True, capture_output=True)

    def command(self, label, args, expect=None, env=None, timeout=120, cwd=None):
        index = len(self.records) + 1
        trace = self.root / "trace.jsonl"
        hooks = self.root / "hooks.log"
        before_trace = trace.read_text().splitlines() if trace.exists() else []
        before_hooks = hooks.read_text().splitlines() if hooks.exists() else []
        command = [self.binary, *args]
        actual_env = dict(self.env)
        actual_env.update(env or {})
        started = time.time()
        process = subprocess.Popen(command, cwd=cwd or self.project, env=actual_env,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        timed_out = False
        try:
            stdout, stderr = process.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(process.pid, signal.SIGTERM)
            try:
                stdout, stderr = process.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                stdout, stderr = process.communicate()
        prefix = self.logs / f"{index:03d}-{label}"
        prefix.with_suffix(".stdout").write_bytes(stdout)
        prefix.with_suffix(".stderr").write_bytes(stderr)
        after_trace = trace.read_text().splitlines() if trace.exists() else []
        after_hooks = hooks.read_text().splitlines() if hooks.exists() else []
        result = {"label": label, "command": command, "cwd": str(cwd or self.project),
                  "environment_overrides": actual_env, "exit_code": process.returncode,
                  "seconds": time.time() - started, "timeout": timed_out,
                  "trace": [json.loads(x) for x in after_trace[len(before_trace):]],
                  "hooks": after_hooks[len(before_hooks):], "stdout": str(prefix.with_suffix('.stdout')),
                  "stderr": str(prefix.with_suffix('.stderr'))}
        self.records.append(result)
        prefix.with_suffix(".json").write_text(json.dumps(result, indent=2))
        if expect is not None:
            self.check(label + " exit", process.returncode in expect, {"actual": process.returncode, "expected": expect})
        self.save()
        print(self.root.name, label, process.returncode, round(result["seconds"], 2), flush=True)
        return result, stdout.decode(errors="replace"), stderr.decode(errors="replace")

    def check(self, name, passed, detail=None):
        self.checks.append({"name": name, "passed": bool(passed), "detail": detail})

    def save(self):
        (self.root / "results.json").write_text(json.dumps({"revision": self.revision, "engine": self.engine,
                                                           "commands": self.records, "checks": self.checks}, indent=2))

    def output(self, label, component="producer", stack="dev"):
        result, stdout, _ = self.command(label, ["terraform", "output", component, "-s", stack, "-json"], [0])
        try:
            parsed = json.loads(stdout)
            self.check(label + " JSON", True)
            return parsed
        except ValueError:
            self.check(label + " JSON", False, stdout[:250])
            return {}

    def state_snapshot(self):
        return {str(p.relative_to(self.project)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in self.project.rglob("*.tfstate") if ".terraform" not in p.parts}


def lifecycle(case):
    c = case
    c.command("config", ["describe", "config", "-f", "json"], [0])
    c.command("explicit-init", ["terraform", "init", "producer", "-s", "dev"], [0])
    initial, _, _ = c.command("initial-plan", ["terraform", "plan", "producer", "-s", "dev", "-detailed-exitcode"], [2])
    if initial["exit_code"] != 2:
        raise RuntimeError("Initial fixture plan failed; inspect evidence before continuing")
    c.command("apply-saved", ["terraform", "apply", "producer", "-s", "dev", "--from-plan"], [0])
    first = c.output("first-output")
    c.check("initial output value", first.get("name", {}).get("value") == "dev-producer-v1", first)
    c.command("warm-plan", ["terraform", "plan", "producer", "-s", "dev", "-detailed-exitcode"], [0])
    c.command("state-list", ["terraform", "state", "list", "producer", "-s", "dev"], [0])
    c.command("prod-apply", ["terraform", "apply", "producer", "-s", "prod", "-auto-approve"], [0])
    c.output("prod-output", stack="prod")
    c.output("dev-after-prod")
    c.command("bulk-apply", ["terraform", "apply", "--all", "-s", "dev", "-auto-approve", "--max-concurrency=1"], [0])
    for component in ("consumer", "state-consumer"):
        out = c.output(component + "-output", component)
        c.check(component + " initial upstream", out.get("upstream", {}).get("value") == "dev-producer-v1", out)
    dev = c.project / "stacks/deploy/dev.yaml"
    dev.write_text(dev.read_text().replace("dev-producer-v1", "dev-producer-v2"))
    c.command("bulk-update", ["terraform", "apply", "--all", "-s", "dev", "-auto-approve", "--max-concurrency=1"], [0])
    for component in ("consumer", "state-consumer"):
        out = c.output(component + "-updated", component)
        c.check(component + " fresh upstream", out.get("upstream", {}).get("value") == "dev-producer-v2", out)
    selectors = {"tags": ["--tags=other"], "labels": ["--labels=team=platform"],
                 "query": ["--query", '.metadata.labels.tier == "data"'],
                 "components": ["--components=consumer", "--include-dependencies"],
                 "tags-labels": ["--tags=other", "--labels=team=application"],
                 "tags-dependencies": ["--tags=data", "--include-dependencies"],
                 "all-tags": ["--all", "--tags=other"], "stack-only": [],
                 "all-parallel": ["--all", "--max-concurrency=2", "--log-order=grouped"]}
    for label, flags in selectors.items():
        c.command("select-" + label, ["terraform", "plan", "-s", "dev", *flags], [0])
    c.command("saved-v2", ["terraform", "plan", "producer", "-s", "dev"], [0])
    dev.write_text(dev.read_text().replace("dev-producer-v2", "dev-producer-v3"))
    c.command("apply-old-saved", ["terraform", "apply", "producer", "-s", "dev", "--from-plan"], [0])
    out = c.output("saved-input-output")
    c.check("saved plan preserves old inputs", out.get("name", {}).get("value") == "dev-producer-v2", out)
    c.command("hooks-disabled", ["terraform", "output", "producer", "-s", "dev", "-json", "--skip-hooks"], [0])
    c.check("skip-hooks silent", not c.records[-1]["hooks"], c.records[-1]["hooks"])
    c.command("test", ["terraform", "test", "producer", "-s", "dev"], [0])
    c.command("refresh", ["terraform", "refresh", "producer", "-s", "dev"], [0])
    c.command("deploy", ["terraform", "deploy", "producer", "-s", "dev"], [0])
    before = c.state_snapshot()
    c.command("policy-denied-destroy-plan", ["terraform", "plan", "producer", "-s", "dev", "-destroy"], [77],
              env={"ATMOS_POLICY_DIR": str(c.project / "policy")})
    c.check("denied plan preserves state", c.state_snapshot() == before)
    c.command("policy-denied-apply", ["terraform", "apply", "producer", "-s", "dev", "-auto-approve"], [1],
              env={"ATMOS_POLICY_DIR": str(c.project / "policy")})
    c.check("denied apply preserves state", c.state_snapshot() == before)
    c.command("bulk-destroy", ["terraform", "destroy", "--all", "-s", "dev", "-auto-approve", "--max-concurrency=1"], [0])
    out = c.output("prod-after-dev-destroy", stack="prod")
    c.check("other stack survives destroy", out.get("name", {}).get("value") == "prod-producer-v1", out)
    c.command("prod-destroy", ["terraform", "destroy", "producer", "-s", "prod", "-auto-approve"], [0])
    c.save()


if __name__ == "__main__":
    root, baseline, candidate = sys.argv[1:]
    for engine in ("terraform", "tofu"):
        for revision, binary in (("baseline", baseline), ("candidate", candidate)):
            lifecycle(Case(Path(root) / f"{engine}-{revision}", revision, engine, binary))
