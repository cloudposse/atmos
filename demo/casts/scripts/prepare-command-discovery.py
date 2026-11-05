"""Prepare an offline Git source containing the actual recommended example."""

import json
import shutil
import subprocess
from pathlib import Path

repo = Path("../..").resolve()
fixture = repo / ".context/cast-fixtures/command-discovery"
if fixture.exists():
    shutil.rmtree(fixture)
source = fixture / "repository"
project = fixture / "project"
project.mkdir(parents=True)
shutil.copytree(
    repo / "examples/quick-start-simple",
    source / "examples/quick-start-simple",
    ignore=shutil.ignore_patterns(".terraform", "*.tfstate*", ".DS_Store"),
)
for args in [
    ["init", "-q"],
    ["add", "."],
    ["-c", "user.name=Atmos Demo", "-c", "user.email=demo@example.invalid", "commit", "-qm", "Quick Start fixture"],
]:
    subprocess.run(["git", "-C", str(source), *args], check=True)
(project / "atmos.yaml").write_text(
    "init:\n  repository: " + json.dumps("git::" + source.as_uri()) + "\n  depth: 0\n  git: false\n"
)
# Read by the sanitizer; the source example itself is copied without edits.
(fixture / "project-path.txt").write_text(str(project))
