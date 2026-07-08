# Demo: Atmos Version Tracker

This example demonstrates the [Atmos Version Tracker](https://atmos.tools/cli/commands/version/track): one catalog of external versions declared in `atmos.yaml`, resolved deterministically into `versions.lock.yaml`, and applied to project files by the file managers.

## What it shows

| File | Manager | What gets rewritten |
| --- | --- | --- |
| `workflows/ci.yaml` | `github-actions` | The `uses: actions/checkout@...` ref |
| `Dockerfile` | `marker` | The `ENV TOFU_VERSION=...` value under the `# atmos:version opentofu` annotation |
| `versions.json` | `template` | Rendered from `versions.json.tmpl` with `{{ .version.* }}` |

## Try it

```shell
cd examples/demo-version-tracker

# Everything is current: the CI gate passes.
atmos version track verify

# Show the catalog and lock status.
atmos version track status

# Simulate drift, then let the tracker repair it.
sed -i.bak 's/v6.1.0/v4/' workflows/ci.yaml && rm workflows/ci.yaml.bak
atmos version track apply --check   # fails: file out of date
atmos version track apply           # rewrites the ref from the lock
atmos version track verify          # passes again
```

The desired versions in this example are concrete, so every command works offline — no registry or GitHub API access is needed. This example is exercised end-to-end in CI by `.github/workflows/version-tracker.yaml`.
