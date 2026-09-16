#!/usr/bin/env python3
"""Deploy a static site to S3 without rewriting unchanged objects."""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import json
import mimetypes
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from typing import Any
from urllib.parse import urlparse


MANIFEST_NAME = ".cloudposse-deploy-manifest-v1.json"
TEXT_TYPES = {
    ".html": "text/html; charset=utf-8",
    ".htm": "text/html; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".js": "application/javascript; charset=utf-8",
    ".mjs": "application/javascript; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".map": "application/json; charset=utf-8",
    ".webmanifest": "application/manifest+json; charset=utf-8",
    ".xml": "application/xml; charset=utf-8",
    ".rss": "application/xml; charset=utf-8",
    ".atom": "application/xml; charset=utf-8",
    ".svg": "image/svg+xml; charset=utf-8",
    ".txt": "text/plain; charset=utf-8",
    ".md": "text/markdown; charset=utf-8",
    ".csv": "text/plain; charset=utf-8",
    ".tsv": "text/plain; charset=utf-8",
    ".yaml": "text/plain; charset=utf-8",
    ".yml": "text/plain; charset=utf-8",
    ".sh": "text/x-shellscript; charset=utf-8",
    ".bash": "text/x-shellscript; charset=utf-8",
    ".tf": "text/plain; charset=utf-8",
    ".tfvars": "text/plain; charset=utf-8",
    ".hcl": "text/plain; charset=utf-8",
    ".rego": "text/plain; charset=utf-8",
    ".toml": "application/toml; charset=utf-8",
    ".ini": "text/plain; charset=utf-8",
    ".cfg": "text/plain; charset=utf-8",
    ".py": "text/plain; charset=utf-8",
    ".go": "text/plain; charset=utf-8",
    ".rb": "text/plain; charset=utf-8",
    ".ts": "text/plain; charset=utf-8",
    ".tsx": "text/plain; charset=utf-8",
    ".jsx": "text/plain; charset=utf-8",
}


def run_aws(*args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(["aws", *args], capture_output=True, text=True)
    if check and result.returncode != 0:
        print(result.stdout, file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        raise subprocess.CalledProcessError(result.returncode, result.args)
    return result


def normalize_s3_uri(uri: str) -> str:
    return uri if uri.endswith("/") else f"{uri}/"


def parse_s3_uri(uri: str) -> tuple[str, str]:
    parsed = urlparse(uri)
    if parsed.scheme != "s3" or not parsed.netloc:
        raise ValueError(f"invalid S3 URI: {uri}")
    return parsed.netloc, parsed.path.lstrip("/").rstrip("/")


def content_type(relative_path: str) -> str:
    explicit = TEXT_TYPES.get(Path(relative_path).suffix.lower())
    if explicit:
        return explicit
    guessed, _ = mimetypes.guess_type(relative_path)
    return guessed or "application/octet-stream"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def build_manifest(local_dir: Path) -> dict[str, Any]:
    files: dict[str, Any] = {}
    for path in sorted(local_dir.rglob("*")):
        if not path.is_file():
            continue
        relative = path.relative_to(local_dir).as_posix()
        if relative == MANIFEST_NAME:
            continue
        files[relative] = {
            "sha256": sha256(path),
            "size": path.stat().st_size,
            "content_type": content_type(relative),
        }
    return {"version": 1, "files": files}


def is_protected(relative_path: str, protected_patterns: tuple[str, ...]) -> bool:
    return any(fnmatch.fnmatchcase(relative_path, pattern) for pattern in protected_patterns)


def manifest_diff(
    old_manifest: dict[str, Any],
    new_manifest: dict[str, Any],
    protected_patterns: tuple[str, ...] = (),
) -> tuple[list[str], list[str]]:
    old_files = old_manifest.get("files", {})
    new_files = new_manifest.get("files", {})
    changed = sorted(
        relative for relative, metadata in new_files.items()
        if old_files.get(relative) != metadata
    )
    deleted = sorted(
        relative for relative in old_files
        if relative not in new_files and not is_protected(relative, protected_patterns)
    )
    return changed, deleted


def load_remote_manifest(s3_uri: str, destination: Path) -> dict[str, Any] | None:
    result = run_aws(
        "s3", "cp", f"{s3_uri}{MANIFEST_NAME}", str(destination),
        "--only-show-errors", check=False,
    )
    if result.returncode != 0:
        if "404" in result.stderr or "Not Found" in result.stderr or "NoSuchKey" in result.stderr:
            return None
        print(result.stderr, file=sys.stderr)
        raise subprocess.CalledProcessError(result.returncode, result.args)
    with destination.open(encoding="utf-8") as stream:
        manifest = json.load(stream)
    if manifest.get("version") != 1 or not isinstance(manifest.get("files"), dict):
        raise ValueError("unsupported or invalid remote deployment manifest")
    return manifest


def write_manifest(manifest: dict[str, Any], destination: Path) -> None:
    with destination.open("w", encoding="utf-8") as stream:
        json.dump(manifest, stream, sort_keys=True, separators=(",", ":"))
        stream.write("\n")


def bootstrap(
    local_dir: Path, s3_uri: str, protected_patterns: tuple[str, ...]
) -> None:
    print("::group::Bootstrap S3 deployment manifest")
    print("No remote manifest found; performing the one-time full metadata sync.")
    sync_args = ["s3", "sync", str(local_dir), s3_uri, "--delete"]
    for pattern in protected_patterns:
        sync_args.extend(("--exclude", pattern))
    run_aws(*sync_args, "--only-show-errors")
    for extension, mime in sorted(TEXT_TYPES.items()):
        run_aws(
            "s3", "cp", s3_uri, s3_uri, "--recursive",
            "--exclude", "*", "--include", f"*{extension}",
            "--metadata-directive", "REPLACE", "--content-type", mime,
            "--only-show-errors",
        )
    print("::endgroup::")


def stage_changed_files(
    local_dir: Path, changed: list[str], staging_dir: Path
) -> list[tuple[str, Path]]:
    grouped: dict[str, list[str]] = {}
    for relative in changed:
        grouped.setdefault(content_type(relative), []).append(relative)
    staged_groups: list[tuple[str, Path]] = []
    for index, (mime, paths) in enumerate(sorted(grouped.items())):
        group_dir = staging_dir / f"group-{index}"
        staged_groups.append((mime, group_dir))
        for relative in paths:
            source = local_dir / relative
            destination = group_dir / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            try:
                os.link(source, destination)
            except OSError:
                shutil.copy2(source, destination)
    return staged_groups


def upload_changed(staged_groups: list[tuple[str, Path]], s3_uri: str) -> None:
    for mime, group_dir in staged_groups:
        run_aws(
            "s3", "cp", str(group_dir), s3_uri, "--recursive",
            "--content-type", mime, "--only-show-errors",
        )


def delete_removed(bucket: str, prefix: str, deleted: list[str], temp_dir: Path) -> None:
    for offset in range(0, len(deleted), 1000):
        batch = deleted[offset:offset + 1000]
        objects = [
            {"Key": f"{prefix}/{relative}" if prefix else relative}
            for relative in batch
        ]
        request_path = temp_dir / f"delete-{offset // 1000}.json"
        with request_path.open("w", encoding="utf-8") as stream:
            json.dump({"Objects": objects, "Quiet": True}, stream)
        run_aws(
            "s3api", "delete-objects", "--bucket", bucket,
            "--delete", f"file://{request_path}",
        )


def upload_manifest(manifest_path: Path, s3_uri: str) -> None:
    run_aws(
        "s3", "cp", str(manifest_path), f"{s3_uri}{MANIFEST_NAME}",
        "--content-type", "application/json; charset=utf-8", "--only-show-errors",
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("local_dir")
    parser.add_argument("s3_uri")
    parser.add_argument(
        "--protect",
        action="append",
        default=[],
        help="Remote-relative glob to exclude from deletion; repeat as needed",
    )
    args = parser.parse_args()
    local_dir = Path(args.local_dir).resolve()
    if not local_dir.is_dir():
        print(f"local directory does not exist: {local_dir}", file=sys.stderr)
        return 2
    s3_uri = normalize_s3_uri(args.s3_uri)
    bucket, prefix = parse_s3_uri(s3_uri)
    protected_patterns = tuple(args.protect)

    print(f"::group::Build content manifest for {local_dir}")
    new_manifest = build_manifest(local_dir)
    print(f"Managed files: {len(new_manifest['files'])}")
    print("::endgroup::")

    with tempfile.TemporaryDirectory(prefix="s3-deploy-") as temp_name:
        temp_dir = Path(temp_name)
        manifest_path = temp_dir / MANIFEST_NAME
        old_manifest = load_remote_manifest(s3_uri, manifest_path)
        write_manifest(new_manifest, manifest_path)

        if old_manifest is None:
            bootstrap(local_dir, s3_uri, protected_patterns)
            upload_manifest(manifest_path, s3_uri)
            print(f"Bootstrapped {len(new_manifest['files'])} managed objects.")
            return 0

        changed, deleted = manifest_diff(
            old_manifest, new_manifest, protected_patterns
        )
        print("::group::Incremental S3 deployment")
        print(f"Changed/new: {len(changed)}; deleted: {len(deleted)}")
        if not changed and not deleted:
            print("No content changes; zero S3 writes required.")
            print("::endgroup::")
            return 0

        if changed:
            staging_dir = temp_dir / "changed"
            staging_dir.mkdir()
            staged_groups = stage_changed_files(local_dir, changed, staging_dir)
            upload_changed(staged_groups, s3_uri)
        if deleted:
            delete_removed(bucket, prefix, deleted, temp_dir)
        upload_manifest(manifest_path, s3_uri)
        print("::endgroup::")
    return 0


if __name__ == "__main__":
    sys.exit(main())
