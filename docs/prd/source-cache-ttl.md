# PRD: Source Cache TTL

**Status:** Implemented
**Version:** 1.0
**Last Updated:** 2026-03-03
**Author:** Erik Osterman

---

## Executive Summary

Add a `ttl` (time-to-live) field to component source configuration that controls how long cached JIT-vendored sources are reused before re-pulling from the remote. This enables automatic refresh of components using floating refs (branches, moving tags) without manual intervention, while maintaining backward compatibility by defaulting to infinite cache.

**Key Principle:** Cache expiration is declarative policy, not an imperative action. Users describe how stale is acceptable, and the system enforces it.

---

## Problem Statement

When JIT-vendored components use floating refs (e.g., `version: "main"`), Atmos skips re-pulling because the version string in workdir metadata hasn't changed — it's still `"main"` even though the upstream commit changed. This creates a stale cache problem during:

1. **Active development** — Developers push to a branch, run `atmos terraform plan`, and get stale code.
2. **Team collaboration** — Multiple engineers iterating on the same module expect pushes to be picked up.
3. **CI/local parity** — CI runs in ephemeral environments (always fresh), but local environments cache indefinitely.

The only workarounds today are:
- Manually delete `.workdir/` before every plan.
- Run `atmos terraform source pull --force` as a separate step.
- Remember to do either — which developers won't.

---

## Solution

Add a `ttl` field to the source specification. When set, the source provisioner compares the workdir's `UpdatedAt` timestamp against the TTL duration. If expired, the source is re-pulled automatically.

### Behavior

| TTL Value | Behavior |
|-----------|----------|
| Not set | Infinite cache — only re-pull on version/URI changes (current behavior) |
| `"0s"` | Always expired — always re-pull on every command |
| `"1h"` | Re-pull if last update was more than 1 hour ago |
| `"7d"` | Re-pull if last update was more than 7 days ago |

---

## Configuration

### Per-Component (Stack Manifest)

```yaml
components:
  terraform:
    my-module:
      source:
        uri: "git::https://github.com/org/repo.git"
        version: "main"
        ttl: "0s"          # Always re-pull (active development)
```

### Global Default (atmos.yaml)

```yaml
components:
  terraform:
    source:
      ttl: "1h"            # Default: re-pull if older than 1 hour
```

### Composability

Per-component TTL overrides global. Global applies to all components that don't set their own TTL. Omitting TTL everywhere preserves current behavior (infinite cache).

---

## Architecture

### Existing Infrastructure Leveraged

The workdir system already has all the building blocks:

- **`WorkdirMetadata.UpdatedAt`** — Timestamp of last source update (already tracked).
- **`WorkdirMetadata.LastAccessed`** — Timestamp of last access (already tracked, used for cleanup).
- **`duration.ParseDuration()`** — Parses human-friendly durations: `"0s"`, `"1h"`, `"7d"`, `"daily"`.
- **`findExpiredWorkdirs()`** — Already compares timestamps against TTL (used by `workdir clean --expired`).

### Decision Point

`needsProvisioning()` in `pkg/provisioner/source/provision_hook.go` is the single decision function that determines whether to re-pull. The TTL check is added after the existing version/URI change checks:

```
Directory empty?          → Provision (new)
Metadata missing?         → Provision (fresh workdir)
Version changed?          → Provision (version bump)
URI changed?              → Provision (source moved)
TTL set and expired?      → Provision (cache stale)    ← NEW
Otherwise                 → Skip (cache valid)
```

### Zero TTL Handling

The `duration.ParseDuration()` function rejects zero values by design (used for cleanup where zero makes no sense). For source TTL, zero means "always expired" — a legitimate and common case. This is handled with an explicit `isZeroTTL()` check before parsing.

---

## Files Modified

| File | Change |
|------|--------|
| `pkg/schema/vendor_component.go` | Add `TTL` field to `VendorComponentSource` |
| `pkg/schema/schema.go` | Add `SourceSettings` struct with `TTL`, add `Source` field to `Terraform`, `Helmfile`, and `Packer` |
| `pkg/provisioner/source/extract.go` | Parse `ttl` from source map |
| `pkg/provisioner/source/provision_hook.go` | TTL check in `needsProvisioning()`, global default merging, `isSourceCacheExpired()` helper |

---

## References

- GitHub Issue: #2135
- Provisioner System PRD: `docs/prd/provisioner-system.md`
- Workdir cleanup: `pkg/provisioner/workdir/clean.go`
