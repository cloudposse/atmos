# Fix: Component Auth Default-Only Markers

**Date:** 2026-07-21

**Status:** Implemented

## Summary

A component-level `auth.identities.<name>.default: false` entry is intended to
demote an identity for that component. When the active global/profile auth
configuration does not define `<name>`, the component-auth deep merge currently
creates an identity with an empty `kind`. Auth-manager initialization validates
that unintended identity and fails.

The fix treats an undefined false-only entry as a default marker rather than
an identity declaration.

## Reproduction

```yaml
# Active global/profile auth configuration
auth:
  identities:
    deploy:
      kind: aws/assume-role
      default: true
```

```yaml
# Resolved component auth configuration
auth:
  identities:
    deploy:
      default: true
    audit:
      default: false
```

Today, the component merge adds `audit` to the effective auth configuration
without a `kind`, and initialization reports that `audit` is not configured.

## Root Cause

`pkg/auth/config_helpers.go:MergeComponentAuthConfig` deep-merges all entries
from the component `auth.identities` map into global auth. It has no distinction
between a default-selection marker and an identity declaration. The resulting
`schema.Identity` for an undefined false-only entry has `Default: false` and an
empty `Kind`; `manager.initializeIdentities` correctly rejects that incomplete
identity.

## Intended Behavior

- An undefined entry whose only field is `default: false` is removed before
  the component/global auth merge. It has no runtime identity and does not
  cause an error.
- A false-only entry for an existing global identity remains an override of
  that identity's default state.
- An undefined entry whose only field is `default: true` fails early with
  `ErrInvalidIdentityConfig` and an actionable message, because it attempts to
  select an identity that cannot be used.
- Entries containing `kind` or any field besides `default` remain identity
  declarations and retain normal merge and validation behavior.

See [Default Auth Behavior PRD](../prd/default-auth-behavior.md) for the full
behavioral contract and acceptance criteria.

## Regression Coverage

`TestMergeComponentAuthFromConfig_DefaultOnlyUndefinedIdentityIsIgnored`
asserts that an undefined false-only marker is removed and initialization
succeeds. Companion unit tests cover existing false-only and true-only
markers, an undefined true-only marker that returns `ErrInvalidIdentityConfig`
without mutating global defaults, and incomplete non-marker declarations.
