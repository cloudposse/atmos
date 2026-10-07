"""Exercise the complete local pipeline, including repeated and changed releases."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
BUILD = ROOT / "components/cloudformation/app/.build"
BUCKET = "atmos-cfn-advanced-artifacts-local"
ENV = {**os.environ, "ATMOS_DEMO_RELEASE": "v1", "AWS_PAGER": ""}


def atmos(*args, capture=False):
    print("+ atmos", *args, flush=True)
    return subprocess.run(
        ["atmos", *args], cwd=ROOT, env=ENV, check=True,
        text=True, stdout=subprocess.PIPE if capture else None,
    ).stdout


def aws(*args, capture=False):
    return atmos("auth", "exec", "--identity", "local-aws", "--", "aws", *args,
                 capture=capture)


def deploy(release, changed, unchanged):
    ENV["ATMOS_DEMO_RELEASE"] = release
    atmos("aws", "cfn", "deploy", "app", "-s", "advanced")
    result = json.loads((BUILD / "publish.json").read_text())
    assert result == {"changed": changed, "unchanged": unchanged}, result
    # Verify the bytes in S3, in addition to the HTTP response checked by the hook.
    downloaded = BUILD / "downloaded.zip"
    aws("s3api", "get-object", "--bucket", BUCKET, "--key",
        f"lambda/{release}/handler.zip", str(downloaded))
    expected = (BUILD / "handler.zip").read_bytes()
    assert downloaded.read_bytes() == expected, "Published archive differs from local build"
    print(f"Verified {release}: {changed} uploaded, {unchanged} unchanged", flush=True)
    return hashlib.sha256(expected).hexdigest()


def main():
    for command in ("atmos", "aws", "python3"):
        if shutil.which(command) is None:
            raise SystemExit(f"Missing prerequisite: {command}")
    atmos("validate", "stacks")
    started = False
    cleanup_errors = []
    try:
        # Set before startup so a partially started emulator is also cleaned up.
        started = True
        atmos("emulator", "up", "aws", "-s", "advanced")
        atmos("aws", "cfn", "deploy", "artifacts", "-s", "advanced")
        first = deploy("v1", changed=1, unchanged=0)
        repeated = deploy("v1", changed=0, unchanged=1)
        assert first == repeated, "Unchanged build produced a different archive"
        updated = deploy("v2", changed=1, unchanged=0)
        assert first != updated, "Updated release did not change the package"
        print("PASS: build → test → archive → publish → deploy → HTTP validation", flush=True)
    finally:
        if started:
            # Attempt every cleanup even if deployment or an earlier cleanup failed.
            for command in (
                lambda: atmos("aws", "cfn", "delete", "app", "-s", "advanced", "--auto-approve"),
                lambda: aws("s3", "rm", f"s3://{BUCKET}", "--recursive"),
                lambda: atmos("aws", "cfn", "delete", "artifacts", "-s", "advanced", "--auto-approve"),
                lambda: atmos("emulator", "down", "aws", "-s", "advanced"),
            ):
                try:
                    command()
                except subprocess.CalledProcessError as error:
                    cleanup_errors.append(str(error))
                    print(f"Cleanup failed: {error}", file=sys.stderr)
    if cleanup_errors:
        raise SystemExit("Cleanup failed:\n" + "\n".join(cleanup_errors))


if __name__ == "__main__":
    main()
