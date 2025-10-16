---
slug: pager-default-correction
title: Pager Default Behavior Corrected
authors: [atmos]
tags: [atmos, bugfix, breaking-change]
---

We've identified and corrected a regression in Atmos where the pager was incorrectly enabled by default, contrary to the intended behavior documented in a previous release.

## What Changed

The pager is now correctly **disabled by default** in Atmos. This aligns with the behavior that was intended in PR #1430 (September 2025) but was not fully implemented.

## Background

In May 2025, pager support was added to Atmos with the default set to `true` (enabled). Later, in September 2025, PR #1430 was merged with the intention of changing this default to improve the scripting and automation experience. The PR included:

- A global `--pager` flag
- Support for the `NO_PAGER` environment variable
- Documentation stating: "**BREAKING CHANGE**: Pager is now disabled by default"

However, the actual default value in the configuration system was never changed from `true` to `false`, causing the pager to remain enabled by default despite the documentation.

## Impact

If you've been experiencing unexpected pager behavior (output being displayed through a pager like `less` when you didn't expect it), this fix resolves that issue.

If your workflow relied on the pager being enabled by default, you'll need to explicitly enable it using one of these methods:

### Enable Pager via Configuration

Add to your `atmos.yaml`:

```yaml
settings:
  terminal:
    pager: true
```

### Enable Pager via CLI Flag

Use the `--pager` flag on any command:

```bash
atmos describe component myapp -s prod --pager
```

### Enable Pager via Environment Variable

Set the `ATMOS_PAGER` or `PAGER` environment variable:

```bash
export ATMOS_PAGER=true
atmos describe component myapp -s prod
```

Or specify a custom pager:

```bash
export ATMOS_PAGER=less
atmos describe component myapp -s prod
```

## Why This Change Matters

Having the pager disabled by default provides several benefits:

1. **Better automation/scripting**: Output can be piped and processed without unexpected pager interaction
2. **Predictable behavior**: Commands behave consistently whether run interactively or in CI/CD
3. **Explicit opt-in**: Users who want pagination can easily enable it per their preferences

## Migration Guide

Most users won't need to change anything. If you were relying on the pager being enabled by default:

1. Add `pager: true` to your `atmos.yaml` settings
2. Or use the `--pager` flag when you want paginated output
3. Or set the `ATMOS_PAGER` environment variable in your shell profile

## Related Links

- [PR #1642: Pager Default Correction](https://github.com/cloudposse/atmos/pull/1642)
- [Original PR #1430: Pager Improvements](https://github.com/cloudposse/atmos/pull/1430)
- [Terminal Configuration Documentation](/cli/configuration/terminal)

We apologize for any confusion this regression may have caused and thank the community for bringing it to our attention.
