"""Plan independently versioned plugin releases from committed manifests."""
import argparse
import fnmatch
import json
import re
import subprocess
from pathlib import PurePosixPath

import yaml

NAMESPACE = "oci://ghcr.io/cloudposse/atmos/plugins/"
RUNNERS = {
    "linux/amd64": "ubuntu-24.04",
    "linux/arm64": "ubuntu-24.04-arm",
    "darwin/amd64": "macos-15-intel",
    "darwin/arm64": "macos-15",
    "windows/amd64": "windows-2022",
    "windows/arm64": "windows-11-arm",
}
VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+")


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def document(ref, path):
    return yaml.safe_load(git("show", f"{ref}:{path}"))


def safe_path(path):
    return isinstance(path, str) and path and not PurePosixPath(path).is_absolute() and ".." not in PurePosixPath(path).parts and "\\" not in path


def version(manifest):
    value = str(manifest["metadata"]["version"])
    if not VERSION.fullmatch(value):
        raise ValueError("Official releases require a stable major.minor.patch version")
    return tuple(int(item) for item in value.split("."))


def affected(paths, package):
    return any(path.startswith(package["path"] + "/") or any(fnmatch.fnmatchcase(path, pattern) for pattern in package["inputs"]) for path in paths)


def plan(head, base=None):
    head = git("rev-parse", "--verify", head + "^{commit}")
    inventory = document(head, "plugins/releases.yaml")["packages"]
    changed = git("diff", "--name-only", base, head).splitlines() if base else []
    packages, matrix = [], []
    for name, config in sorted(inventory.items()):
        if not re.fullmatch(r"[a-z][a-z0-9-]*", name) or config["path"] != "plugins/" + name:
            raise ValueError("Package names must match their plugins/<name> directory")
        if not isinstance(config.get("inputs"), list) or not all(safe_path(item) for item in config["inputs"]):
            raise ValueError("Declare repository-relative shared build inputs")
        path = config["path"] + "/plugin.yaml"
        manifest = document(head, path)
        current = version(manifest)
        if manifest["metadata"]["name"] != "cloudposse/" + name:
            raise ValueError("Inventory name differs from package manifest")
        if base and path in git("ls-tree", "-r", "--name-only", base, path).splitlines():
            old = document(base, path)
            if current < version(old):
                raise ValueError(f"{name}: version cannot decrease")
            if (affected(changed, config) or "plugins/releases.yaml" in changed) and current <= version(old):
                raise ValueError(f"{name}: changed build inputs require metadata.version to increase")
        # Reconciliation rebuilds from the exact commit that accepted the manifest.
        # Reject later changed inputs instead of silently republishing different bytes.
        source = git("log", "-1", "--format=%H", head, "--", path)
        subsequent = git("diff", "--name-only", source, head).splitlines()
        if affected(subsequent, config):
            raise ValueError(f"{name}: build inputs changed after its version commit")
        platforms = manifest["spec"]["platforms"]
        if set(platforms) != set(RUNNERS):
            raise ValueError(f"{name}: official releases require all six native platforms")
        entry = {"name": name, "path": config["path"], "version": ".".join(map(str, current)), "source": source, "repository": NAMESPACE + name}
        packages.append(entry)
        for platform, runner in RUNNERS.items():
            executable = platforms[platform]["executable"]
            if not safe_path(executable) or not executable.startswith("bin/"):
                raise ValueError("Executables must be relative paths under bin/")
            matrix.append(dict(entry, platform=platform, runner=runner, artifact=platform.replace("/", "-"), executable=executable))
    if not packages:
        raise ValueError("Release inventory is empty")
    return {"packages": {"include": packages}, "build": {"include": matrix}}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--head", default="HEAD")
    parser.add_argument("--base")
    arguments = parser.parse_args()
    print(json.dumps(plan(arguments.head, arguments.base), separators=(",", ":")))
