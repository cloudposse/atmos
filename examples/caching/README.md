---
title: Terraform Registry Cache
tags: [Components]
cast:
  file: /casts/examples/caching/registry-cache.cast
  title: atmos terraform registry cache
---

# Terraform Registry Cache

This example demonstrates the **Terraform registry cache** — transparent **provider and
module** caching behind a single flag — and **eager multi-platform pre-seeding** of
providers via `atmos terraform cache mirror`.

## What's here

- A tiny `random-pet` component that requires several providers (`hashicorp/random`,
  `null`, `local`, `tls`), so real providers flow through the cache on `init`/`plan`.
- The same component consumes a registry-sourced module
  (`cloudposse/label/null`), so module-registry traffic flows through the cache too.
- `atmos.yaml` with the cache enabled and target `platforms` declared:

  ```yaml
  components:
    terraform:
      platforms:
        - linux_amd64
        - darwin_arm64
        - windows_amd64
      cache:
        enabled: true
  ```

## Try it

```shell
# On macOS/Windows, trust the local cache proxy certificate once.
atmos terraform cache trust

# Plan the component. Providers and the registry module are fetched through the
# ephemeral cache proxy and stored on disk. At the end you'll see a report like:
#   Registry cache: 12 MB downloaded and cached (5 objects)
atmos terraform plan random-pet -s dev

# Simulate a fresh checkout / CI runner so init re-installs from scratch. The
# on-disk cache persists across runs even though the proxy is ephemeral, so this
# second init is served entirely from the cache:
#   Registry cache: 12 MB saved (5 hits)
rm -rf components/terraform/random-pet/.terraform
atmos terraform plan random-pet -s dev

# Inspect the cache.
atmos terraform cache list
atmos terraform cache stats
```

The report has two halves: **saved** is bandwidth served from the cache on hits, and
**downloaded and cached** is what the proxy fetched from upstream and added to the
cache on misses. A run that re-uses an existing `.terraform` (no install) contacts the
proxy for nothing and prints no report.

## Module caching

On `init`, the `cloudposse/label/null` module is resolved through the proxy's module
registry mirror: its **version listing** and **download resolution** route through the
cache and are stored. After a run, `atmos terraform cache stats` / `cache list` show a
non-zero **module** count alongside the providers.

What gets cached depends on how the registry serves the module's content:

- **Registry metadata** (version listings, download resolution) is **always** cached,
  removing registry round-trips on subsequent runs.
- The module **archive** is cached when the registry resolves the download to a plain
  HTTP(S) archive (e.g. a `.tar.gz`/`.zip`).
- **Git-sourced** modules — the common case for the public registry and mono-repos —
  pass through unchanged for now; full content caching for those arrives with the
  upcoming git mirror.

Unlike providers, modules are **not** pre-seeded by `cache mirror` (that wraps
`terraform/tofu providers mirror`, which is providers-only). Modules are cached
**lazily through the proxy** the first time `init` resolves them.

## Pre-seed for multiple platforms

The proxy is **lazy** — it only caches the platform Terraform requests at run time
(your host's). For mixed CI/developer fleets, or to build an **air-gapped bundle**,
pre-seed the cache for every target platform with `cache mirror`, which wraps
`terraform providers mirror` into the same cache directory:

```shell
# Uses components.terraform.platforms from atmos.yaml
atmos terraform cache mirror random-pet -s dev

# Or override the platforms for a one-off
atmos terraform cache mirror random-pet -s dev \
  --platform=linux_amd64 --platform=darwin_arm64

atmos terraform cache list
```

The mirror writes the canonical `filesystem_mirror` layout the proxy already serves,
so the same cache directory works three ways: lazily (proxy), eagerly (mirror), and
offline (`filesystem_mirror`).

## Learn more

- [Registry cache configuration](https://atmos.tools/cli/configuration/components/terraform#cache)
- [`atmos terraform cache`](https://atmos.tools/cli/commands/terraform/cache)
- [`atmos terraform cache mirror`](https://atmos.tools/cli/commands/terraform/cache/mirror)
