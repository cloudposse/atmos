"""Reconcile the catalog from accepted main manifests while the workflow holds its lock."""
import argparse
import json
import subprocess
import urllib.error

from plan import NAMESPACE, plan
from public import anonymous_pull, require_public


def reconcile(atmos):
    # This happens after acquiring the catalog job's global concurrency lock.
    subprocess.run(["git", "fetch", "origin", "main"], check=True)
    accepted = plan("origin/main")
    catalog = NAMESPACE + "catalog"
    require_public(catalog)
    added = []
    for package in accepted["packages"]["include"]:
        source, version = package["repository"], package["version"]
        require_public(source)
        try:
            digest = anonymous_pull(source, version)
        except urllib.error.HTTPError as error:
            if error.code == 404:
                # Another accepted package may still be building. Its successful
                # publisher (or workflow_dispatch) will reconcile again.
                continue
            raise
        subprocess.run([atmos, "plugin", "catalog", "add", "cloudposse", source + "@" + digest], check=True)
        added.append(dict(name=package["name"], version=version, digest=digest))
    if not added:
        raise RuntimeError("No accepted public releases are available for the catalog")
    anonymous_pull(catalog, "stable")
    return added


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--atmos", required=True)
    print(json.dumps(reconcile(parser.parse_args().atmos)))
