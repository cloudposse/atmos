"""Fail closed unless GHCR visibility is public and OCI bytes are anonymously readable."""
import argparse
import hashlib
import json
import re
import subprocess
import urllib.parse
import urllib.request

PREFIX = "oci://ghcr.io/cloudposse/atmos/plugins/"
ACCEPT = ", ".join(("application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json"))


def repository(source):
    if not source.startswith(PREFIX) or not re.fullmatch(r"[a-z][a-z0-9-]*", source[len(PREFIX):]):
        raise ValueError("Only official plugin repositories may be published")
    return source.removeprefix("oci://ghcr.io/")


def require_public(source):
    path = repository(source)
    package = urllib.parse.quote(path.removeprefix("cloudposse/"), safe="")
    try:
        result = subprocess.check_output(["gh", "api", f"orgs/cloudposse/packages/container/{package}"], text=True)
    except subprocess.CalledProcessError as error:
        raise RuntimeError(f"Provision {path} as a PUBLIC GHCR package before releasing; see plugins/README.md") from error
    if json.loads(result).get("visibility") != "public":
        raise RuntimeError(f"Refusing to publish: {path} must be public in GitHub package settings")


def fetch(url, headers=None, limit=8 << 20):
    # No Docker configuration, GitHub token, or credential helper is consulted.
    request = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(request, timeout=60) as response:
        data = response.read(limit + 1)
    if len(data) > limit:
        raise ValueError("OCI object exceeds verification limit")
    return data


def anonymous_pull(source, reference, expected=None):
    path = repository(source)
    if not re.fullmatch(r"(?:[0-9]+\.[0-9]+\.[0-9]+|stable|sha256:[0-9a-f]{64})", reference):
        raise ValueError("Invalid release reference")
    query = urllib.parse.urlencode({"service": "ghcr.io", "scope": f"repository:{path}:pull"})
    token = json.loads(fetch("https://ghcr.io/token?" + query))["token"]
    headers = {"Authorization": "Bearer " + token, "Accept": ACCEPT}
    visited = set()

    def pull(ref, kind="manifests", digest=None):
        if digest and not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
            raise ValueError("Unsupported OCI digest")
        if ref in visited:
            return ref
        if len(visited) >= 128:
            raise ValueError("Too many OCI objects")
        visited.add(ref)
        data = fetch(f"https://ghcr.io/v2/{path}/{kind}/{ref}", headers, 256 << 20 if kind == "blobs" else 8 << 20)
        actual = "sha256:" + hashlib.sha256(data).hexdigest()
        if digest and actual != digest:
            raise ValueError("Anonymous OCI digest mismatch")
        if kind == "manifests":
            manifest = json.loads(data)
            if "manifests" in manifest:
                for child in manifest["manifests"]:
                    pull(child["digest"], digest=child["digest"])
            else:
                for blob in [manifest["config"], *manifest["layers"]]:
                    pull(blob["digest"], "blobs", blob["digest"])
        return actual

    return pull(reference, digest=expected)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source")
    parser.add_argument("--reference")
    parser.add_argument("--digest")
    arguments = parser.parse_args()
    require_public(arguments.source)
    if arguments.reference:
        print(anonymous_pull(arguments.source, arguments.reference, arguments.digest))
