---
name: atmos-cache
description: "Atmos caching: CI cache configuration and commands, GitHub Actions cache integration, Terraform registry cache mirror/list/prune/stats/trust, and cache modernization guidance"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Cache

Use this skill for Atmos cache behavior. There are two distinct cache surfaces: CI cache and
Terraform registry cache.

## Related Skills

| Need | Load |
|---|---|
| Native CI workflows | [atmos-ci](../atmos-ci/SKILL.md) |
| Tool versions installed into cache roots | [atmos-toolchain](../atmos-toolchain/SKILL.md) |
| Terraform component execution | [atmos-terraform](../atmos-terraform/SKILL.md) |

## CI Cache

The CI cache restores and saves the Atmos XDG cache root, including toolchain installs and other
known cache directories. GitHub Actions is the supported CI provider today.

```yaml
ci:
  cache:
    enabled: true
    auto: true
    paths:
      - .terraform
    key: atmos-${{ runner.os }}-${{ hashFiles('**/.tool-versions') }}
    restore_keys:
      - atmos-${{ runner.os }}-
```

Commands:

| Command | Purpose |
|---|---|
| `atmos ci cache paths` | Print cache key, paths, and restore keys for `actions/cache` |
| `atmos ci cache restore` | Restore the well-known cache directory |
| `atmos ci cache save` | Save the well-known cache directory |
| `atmos ci cache list` | List CI cache entries |
| `atmos ci cache delete <key>` | Delete a CI cache entry |

Use `cloudposse/atmos/actions/cache@v1` when the workflow wants a composite action wrapper around
Atmos cache behavior. Otherwise use `atmos ci cache paths` with `actions/cache`.

## Terraform Registry Cache

`atmos terraform cache` manages the Atmos Terraform registry cache. It is not Terraform's provider
plugin cache (`TF_PLUGIN_CACHE_DIR`).

Commands:

| Command | Purpose |
|---|---|
| `atmos terraform cache list` | List cached providers/modules |
| `atmos terraform cache mirror` | Pre-seed provider artifacts for multiple platforms |
| `atmos terraform cache stats` | Show cache size and object counts |
| `atmos terraform cache prune` | Remove stale metadata |
| `atmos terraform cache delete <key>` | Delete a cached object |
| `atmos terraform cache trust` | Trust the cache proxy certificate |
| `atmos terraform cache untrust` | Remove the trusted certificate |

Use `mirror` before offline or high-fanout CI runs that need predictable provider availability.
Use `--all`, `--components`, or `--query` filters when warming only part of the project.

## Guidance

- Do not conflate CI cache, Terraform registry cache, and Terraform plugin cache.
- Prefer cache keys that include Atmos/toolchain config and lock files.
- Keep cache restores before toolchain-heavy steps and saves after all Atmos commands complete.
- For private registry or GitHub reads, solve auth first with Atmos Auth or `github/sts`; cache
  should not be used as an auth workaround.
