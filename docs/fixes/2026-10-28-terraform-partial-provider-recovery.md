# Fix: Recover incomplete Terraform provider installations

**Date:** 2026-10-28

## Summary

Smart init detects missing provider executables and unusable package links even when the provider directory still exists. Terraform workspace selection and OpenTofu provider-install diagnostics now receive the same bounded init recovery as plan/apply.

## Context

Field tests reproduced this on current main and the reconciled component-provider extraction. Removing only a provider executable left the old directory-existence precondition satisfied. Terraform could fail during workspace selection before reaching plan recovery; OpenTofu reported a checksum/install diagnostic that was not classified. Init itself could then fail on an incomplete local package, a dangling symlink, or an incomplete shared plugin-cache package.

## Changes

- Check locked provider packages and executable availability for the current platform, with valid same-OS alternate architectures supported for emulated engines, including versioned and unversioned executable names, registry and legacy layouts, symlinks, and `TF_DATA_DIR`. Package contents are not hashed on every warm command.
- Once initialization is authorized, preserve incomplete owned modern package directories under `atmos-incomplete-provider-*` in the data directory and unlink unusable installation references. Shared target contents, valid packages, and aggregate legacy plugin directories are preserved.
- When a shared package is incomplete, scope `TF_PLUGIN_CACHE_DIR` to a persistent `atmos-provider-recovery` directory under this component's data directory for init. Its installed links remain valid afterward; ordinary configuration and warm fingerprints remain unchanged. This environment override also takes precedence over an RC `plugin_cache_dir`.
- Retry workspace initialization once using existing opt-out and upgrade policies. Unknown provider, authentication, and state-lock errors do not authorize recovery. Failed init still invalidates its marker.
- Preserve existing modified executables: a checksum diagnostic permits one init attempt, not blanket deletion of locally modified packages. OpenTofu can still reject such a package.

This repairs existing initialization behavior without changing defaults, configuration semantics, or requiring an edition migration. Recovery artifacts are regenerable provider data and are removed with the Terraform data directory by normal cleanup.

## Validation

Failing behavior tests were run before implementation. The autoinit, RC, and cache package suites passed; the full Terraform component short suite passed. Race tests passed for autoinit and managed RC registration/cleanup. Unit cases cover native/alternate-architecture executable/platform/version selection, valid/broken links, outside-target preservation, permission failures, environment isolation, opt-outs, unrelated errors, and a persistent failure's single retry.

Real Terraform 1.15.6 and OpenTofu 1.12.5 each passed two fresh native runs with plugin cache enabled and disabled, filesystem-only mirrors, default/custom data directories, missing executables, broken package links, warm runs, and skip/never/upgrade-never controls. Shared-cache artifact hashes and local state bytes stayed unchanged; actual engine argv confirmed bounded recovery. A modified OpenTofu executable retained its deliberate installation error after one attempt. Disposable Linux checks covered both tools and all four plugin/registry-cache combinations.

## Follow-ups

The broader field-findings repair plan tracks combined acceptance and repository-wide validation. Cross-architecture engine binaries running under emulation were not field-tested. No universal scheduling guarantee is added for independent processes modifying the same component directory.
