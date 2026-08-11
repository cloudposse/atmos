---
title: Version Tracker
tags: [Automation]
description: >-
  One catalog of external versions, two independent environment tracks across
  three real ecosystems, and a stack that reads locked versions straight into
  component vars.
cast:
  file: /casts/examples/version-tracker/tracks-and-vars.cast
  title: atmos version tracker — independent dev/prod tracks
---

# Version Tracker

This example demonstrates the [Atmos Version Tracker](https://atmos.tools/cli/commands/version/track): one catalog of external versions declared in `atmos.yaml`, resolved deterministically into `versions.lock.yaml`.

## What it shows

### 1. File managers rewrite pinned versions from the lock

| File | Manager | What gets rewritten |
| --- | --- | --- |
| `workflows/ci.yaml` | `github-actions` | The `uses: actions/checkout@...` ref |
| `Dockerfile` | `marker` | The `ENV TOFU_VERSION=...` value under the `# atmos:version opentofu` annotation |
| `versions.json` | `template` | Rendered from `versions.json.tmpl` with `{{ .version.* }}` |

These three dependencies (`checkout`, `opentofu`, `nginx`) are pinned identically across every track — they're managed by rewriting project files, not by environment.

### 2. Dev and prod are independent version tracks

`stacks/deploy/dev.yaml` and `stacks/deploy/prod.yaml` each assert their own `version.track`, and read locked versions straight into the `app` component's vars with `!version` and `{{ .version.* }}` — no file rewriting involved. Each dependency lives on a different real ecosystem, so the same mechanism works uniformly across all of them:

| Dependency | Ecosystem | `dev` desired | `prod` desired |
| --- | --- | --- | --- |
| `kubectl` | `toolchain` | `1.31.0` | `1.29.4` |
| `redis` | `oci` | `7.4.0` | `7.2.5` |
| `setup_node` | `github-actions` | `v4.1.0` | `v4.0.0` |

Tracks inherit the base dependency catalog and only override what differs, so `checkout`/`opentofu`/`nginx` show up identically under every track in `versions.lock.yaml`.

## Try it

```shell
cd examples/version-tracker

# --- Part 1: file managers rewrite project files from the lock ---

# Everything is current: the CI gate passes.
atmos version track verify

# Show the catalog and lock status.
atmos version track status

# Simulate a hand-edited file, then let the tracker repair it.
sed -i.bak 's/v6.1.0/v4/' workflows/ci.yaml && rm workflows/ci.yaml.bak
atmos version track apply --check   # fails: file out of date
atmos version track apply           # rewrites the ref from the lock
atmos version track verify          # passes again

# --- Part 2: dev and prod are independent tracks, across three ecosystems ---

# One matrix, one command: compare desired/locked versions across every track.
atmos version track list

# The same dependencies, different values, straight from each stack's vars.
atmos describe component app -s dev  --query .vars
atmos describe component app -s prod --query .vars

# Bump dev's redis version with a format-preserving config edit — no sed.
atmos version track set redis --desired=7.4.1 --track=dev
atmos version track lock dev

# Only the dev track's lock entry changed — prod's block is untouched.
git diff versions.lock.yaml

# The new value flows straight through to dev's component vars...
atmos describe component app -s dev  --query .vars.redis_image   # redis:7.4.1
# ...while prod is provably unaffected.
atmos describe component app -s prod --query .vars.redis_image   # still redis:7.2.5

# Restore the example to its committed state.
git checkout -- atmos.yaml versions.lock.yaml
```

The desired versions in this example are concrete, so every command works offline — no registry or GitHub API access is needed. This example is exercised end-to-end in CI by `.github/workflows/version-tracker.yaml`.
