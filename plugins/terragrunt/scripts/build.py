#!/usr/bin/env python3
"""Build and test the native plugin with its pinned, locally maintained facade."""
import argparse
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument("--test", action="store_true", help="Run the plugin and facade tests after building")
args = parser.parse_args()
package = Path(__file__).resolve().parents[1]
repository = package.parents[1]
work = repository / ".context" / "terragrunt-plugin-build"
work.mkdir(parents=True, exist_ok=True)


def go(*arguments, cwd=package, capture=False):
    return subprocess.run(["go", *arguments], cwd=cwd, check=True,
                          text=True, capture_output=capture).stdout


platform = json.loads(go("env", "-json", "GOOS", "GOARCH", capture=True))
suffix = ".exe" if platform["GOOS"] == "windows" else ""
output = package / "bin" / f"{platform['GOOS']}-{platform['GOARCH']}" / f"plugin{suffix}"
output.parent.mkdir(parents=True, exist_ok=True)
go("build", "-p=2", "-ldflags=-w", "-o", str(output), ".")
print(output, flush=True)
if args.test:
    go("test", "-p=2", "-ldflags=-w", "./...",
       "-coverpkg=./...,github.com/gruntwork-io/terragrunt/pkg/atmosbridge",
       f"-coverprofile={work / 'plugin.cover'}")
    go("test", "-p=2", "-ldflags=-w", "./...",
       f"-coverprofile={work / 'bridge.cover'}", cwd=package / "bridge")
