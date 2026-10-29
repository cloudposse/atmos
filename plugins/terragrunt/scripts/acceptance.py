#!/usr/bin/env python3
"""Exercise the installed native plugin through the real Atmos CLI (local state only)."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument("--atmos", required=True, type=Path)
parser.add_argument("--terraform", default="tofu")
args = parser.parse_args()
atmos = args.atmos.resolve()
package = Path(__file__).resolve().parents[1]
repository = package.parents[1]
work = Path(tempfile.mkdtemp(prefix="terragrunt-native-", dir=repository / ".context"))
print(f"Evidence and local state: {work}", flush=True)

def snapshot(root):
    return {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in root.rglob("*") if p.is_file()}

def run(root, label, *command, revision="r1", success=True):
    environment = dict(os.environ, POC_REVISION=revision, TF_IN_AUTOMATION="true", NO_COLOR="1")
    result = subprocess.run([str(atmos), f"--chdir={root}", *command], env=environment,
                            text=True, capture_output=True, timeout=180)
    (work / f"{root.name}-{label}.log").write_text(result.stdout + result.stderr)
    if (result.returncode == 0) != success:
        raise RuntimeError(f"{label}: exit {result.returncode}\n{result.stdout}\n{result.stderr}")
    return result.stdout

for name, scope, stack, expected in [
    ("advanced", "live/dev/**", "live/dev", ["app", "cache", "database", "network"]),
    ("explicit-stack", "live/**", "live", ["dev/app", "dev/network", "prod/app", "prod/network"]),
]:
    root = work / name
    shutil.copytree(package / "fixtures" / name, root)
    config = {"base_path": str(root), "components": {"terraform": {
        "base_path": "components/terraform", "command": args.terraform,
        "terragrunt": {"enabled": True, "mode": "native", "discovery": {
            "enabled": True, "included_paths": [scope]}}}},
        "plugins": {"packages": {"terragrunt": {"path": str(package)}}}}
    (root / "atmos.yaml").write_text(json.dumps(config, indent=2))
    before = snapshot(root)
    instances = json.loads(run(root, "instances", "list", "instances", "--format=json"))
    assert [row["Component"] for row in instances] == expected, instances
    assert {row["Stack"] for row in instances} == {stack}, instances
    run(root, "stacks", "list", "stacks", "--format=json")
    described = json.loads(run(root, "describe", "describe", "stacks", "--format=json"))
    assert stack in described, described.keys()
    assert before == snapshot(root), "Discovery changed project files"
    if name == "advanced":
        run(root, "mock-plan", "terraform", "plan", "--all", "-s", stack, "--", "-no-color")
    run(root, "apply", "terraform", "apply", "--all", "-s", stack, "--", "-auto-approve", "-no-color")
    plan = run(root, "unchanged", "terraform", "plan", "--all", "-s", stack, "--", "-detailed-exitcode", "-no-color")
    assert plan.count("No changes.") == 4, plan
    state_files = list(root.glob("live/dev/*/state/terraform.tfstate")) if name == "advanced" else list(root.glob("state/*/*.tfstate"))
    assert len(state_files) == 4, state_files
    if name == "advanced":
        run(root, "refresh", "terraform", "apply", "--all", "-s", stack, "--", "-auto-approve", "-no-color", revision="r2")
        states = [json.loads(path.read_text()) for path in state_files]
        app = next(state["outputs"]["value"]["value"] for state in states if state["outputs"]["value"]["value"]["unit"] == "app")
        assert app["upstream"]["database"]["upstream"]["network"]["revision"] == "r2", app
    run(root, "destroy", "terraform", "destroy", "--all", "-s", stack, "--", "-auto-approve", "-no-color")
    for path in state_files:
        assert not json.loads(path.read_text())["resources"], path
    print(f"PASS {name}: inventory, read-only discovery, native apply, unchanged plans, dependency state, reverse destroy", flush=True)
