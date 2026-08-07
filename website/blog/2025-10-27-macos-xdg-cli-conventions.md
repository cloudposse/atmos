---
slug: macos-xdg-cli-conventions
title: 'Breaking Change: macOS Now Uses ~/.config for XDG Paths'
authors:
  - osterman
tags:
  - breaking-change
date: 2025-10-27T12:00:00.000Z
release: v1.196.0
---

You authenticate on your Mac, start Geodesic, and the container cannot see your credentials. Nothing failed and nothing was misconfigured — Geodesic mounts `~/.config`, and Atmos had written to `~/Library/Application Support`. On macOS, Atmos now uses `~/.config`, `~/.cache`, and `~/.local/share`, the same directories every other CLI tool in the toolchain uses.

<!--truncate-->

## The Problem

When Atmos added XDG Base Directory support, it took the defaults from the `adrg/xdg` library, which points at `~/Library/Application Support` on macOS. That is correct behavior for a Mac application. It was the wrong answer for Atmos, and it broke in three ways:

1. **Geodesic incompatibility** — Geodesic mounts `~/.config` by default, not `~/Library/Application Support`. Credentials written outside the mount are invisible inside the container.
2. **Ecosystem inconsistency** — gh, git, packer, stripe, op, kubectl, docker, and terraform all use `~/.config` on macOS. Atmos was the outlier in its own toolchain.
3. **Platform fragmentation** — a path that differs between Linux and macOS makes any shared script, Dockerfile, or runbook conditional.

### CLI tools and GUI applications differ here

The ecosystem has a clear split, and it is not about macOS versus Linux — it is about what kind of program you are.

**CLI tools** use `~/.config`, `~/.cache`, and `~/.local/share` on every platform, macOS included. GitHub CLI, HashiCorp Packer, Stripe CLI, and 1Password CLI all do. The payoff is one path everywhere and paths that survive being mounted into a container.

**GUI applications** — the things that live in `/Applications` — use `~/Library/Application Support` and `~/Library/Caches`, which integrates properly with macOS conventions for shipped apps.

Atmos is a CLI tool, so it should follow CLI conventions.

## The Fix

On **macOS**, the defaults change:

**Before:**
- Config: `~/Library/Application Support/atmos/`
- Cache: `~/Library/Caches/atmos/`
- Data: `~/Library/Application Support/atmos/`

**After:**
- Config: `~/.config/atmos/`
- Cache: `~/.cache/atmos/`
- Data: `~/.local/share/atmos/`

Linux and Windows paths are unchanged. The override applies process-wide, so every part of Atmos resolves the same locations — there is no code path left that writes to the old directories.

Your `~/.config` now sits alongside the rest of your tooling:

```text
~/.config/
├── atmos/          # Atmos (now!)
├── gh/             # GitHub CLI
├── git/            # Git
├── packer/         # HashiCorp Packer
├── stripe/         # Stripe CLI
└── op/             # 1Password CLI
```

## How to Use It

### Most users are not affected

If you are upgrading from a version **prior to v1.195.0**, there is nothing to do:

- Older versions used `~/.aws/atmos/`, the legacy path.
- The `~/Library/Application Support` path was never in a stable release.

### If you ran an unreleased build on macOS

This applies only if you used Atmos auth on macOS from the main branch between v1.195.0 and this release. Pick one of three:

#### Option 1: Use the new path (recommended)

```bash
# Re-login to store credentials in new location
atmos auth login
```

Costs you one login. Leaves nothing behind to explain later.

#### Option 2: Keep the existing location

```bash
# Add to ~/.zshrc or ~/.bash_profile
export ATMOS_XDG_CONFIG_HOME="$HOME/Library/Application Support"
```

This pins **all** Atmos XDG paths — config, cache, and data — to the old location, not just credentials. It also reintroduces the Geodesic problem, since the container still expects `~/.config`. Option 1 is the better trade.

#### Option 3: Move the credentials

```bash
if [ -d "$HOME/Library/Application Support/atmos" ]; then
    mkdir -p ~/.config
    mv "$HOME/Library/Application Support/atmos" ~/.config/
fi
```

Preserves the existing session without a re-login, at the cost of a manual move you have to remember to run on every machine.

### Geodesic configuration

Geodesic sets system-wide XDG environment variables (`XDG_CONFIG_HOME=/etc/xdg_config_home`) that need to be overridden, because `/etc/xdg_config_home` is not one of the mounted directories. Add to your Geodesic Dockerfile:

```dockerfile
# Override Geodesic's system XDG paths to use home directory
ENV ATMOS_XDG_CONFIG_HOME=$HOME/.config
ENV ATMOS_XDG_DATA_HOME=$HOME/.local/share
ENV ATMOS_XDG_CACHE_HOME=$HOME/.cache
```

Geodesic already mounts `~/.aws`, `~/.config`, `~/.ssh`, `~/.kube`, and `~/.terraform.d`, so credentials written to `~/.config` are visible inside the container. See [Configuring Geodesic](/tutorials/configuring-geodesic).

### Checking where your credentials are

```bash
ls -la ~/Library/Application\ Support/atmos
ls -la ~/.config/atmos
```

## References

- [Configuring Geodesic with Atmos Auth](/tutorials/configuring-geodesic)
- [Auth Usage Guide](/cli/commands/auth/usage) — updated with the macOS paths
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [Stack Overflow discussion on XDG equivalents on macOS](https://stackoverflow.com/questions/3373948/equivalents-of-xdg-config-home-and-xdg-data-home-on-mac-os-x)
- [Geodesic](https://github.com/cloudposse/geodesic)

## Get Involved

If this migration leaves you somewhere the three options above do not cover, describe your setup at https://github.com/cloudposse/atmos/issues
