# Drift Detection Dispatched for Components Disabled via `metadata.enabled`

**Date:** 2026-06-03
**Severity:** High — disabled components keep failing scheduled drift detection forever (`dispatchError: "missing_plan_result"`, `drift_status: error`), and the noise never clears because every upload re-asserts the wrong state
**Reproducer:** `pkg/list/list_instances_pro_test.go` (`TestExtractProSettings` — "metadata disabled forces pro and drift off")

---

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bugfix: it makes `atmos list instances --upload` report the state Atmos already
resolves at plan time. There is no new command, flag, or feature to announce — only a correction so
the upload payload stops contradicting the CLI's own enabled determination. Per the repo's label
decision tree that makes it a `patch`, which does not require a `website/blog/` post or a roadmap
milestone. The user-facing reference docs (`settings/pro.mdx`, `list/list-instances.mdx`) are
updated in the same PR because they documented the old (now-incorrect) behavior; the rationale and
design live here alongside the other regression/fix write-ups.

---

## Symptom

Components disabled via `metadata.enabled: false` kept failing scheduled drift detection with
`dispatchError: "missing_plan_result"` and `drift_status: error`, even though multiple upstream PRs
had "disabled" those components:

```yaml
metadata:
  enabled: false
vars:
  enabled: false
```

The Atmos CLI clearly knows not to produce a plan — at plan time it skips the disabled component —
yet Atmos Pro kept dispatching drift on a schedule and recording errors because no plan result ever
arrived.

## Root Cause

The "don't run this component" determination lived **only in the Atmos CLI at plan time** and never
crossed the wire to Atmos Pro.

1. **The CLI's enabled signal is `metadata.enabled`.** `isComponentEnabled`
   (`internal/exec/component_utils.go:8`) treats a component as enabled by default and skips it only
   when `metadata.enabled` is the boolean `false`. That is what made the CLI skip planning.

2. **The Atmos Pro upload contract has no `metadata` field.** `atmos list instances --upload`
   serialized only the raw `settings.pro` block (`extractProSettings` in
   `pkg/list/list_instances.go`). Atmos Pro's ingestion schema accepts `component`, `stack`,
   `component_type`, and `settings.pro` — `metadata.enabled` could not influence ingestion even if
   it had been sent.

3. **Atmos Pro derives its persisted state purely from `settings.pro`.** It stores
   `instances.enabled ← settings.pro.enabled` (default **true**) and
   `instances.drift_enabled ← settings.pro.drift_detection.enabled` (default false), and the drift
   scheduler dispatches on `drift_enabled = true AND enabled = true`. With `metadata.enabled` never
   arriving, a component carrying `settings.pro: {enabled: true, drift_detection: {enabled: true}}`
   was persisted as enabled + drift-enabled and dispatched, regardless of `metadata.enabled: false`.

Inspecting Atmos Pro's persisted instance state confirmed the split was driven entirely by the one
signal Atmos Pro stored: rows with `enabled:true, drift_enabled:true` were dispatched and went to
`error`; rows with `drift_enabled:false` were correctly `disabled`. All of them carried
`metadata.enabled:false` — it moved neither column.

## Fix

Collapse the enabled hierarchy in the Atmos CLI **before upload**, so the single signal Atmos Pro
persists already reflects any outer disable. Precedence (an outer disable forces all inner levels
off):

```
metadata.enabled  >  settings.pro.enabled  >  settings.pro.drift_detection.enabled

effectiveProEnabled   = metadata.enabled && settings.pro.enabled                    // both default true
effectiveDriftEnabled = effectiveProEnabled && settings.pro.drift_detection.enabled // drift defaults false
```

Implementation (`pkg/list/list_instances.go`):

- A single `effectiveEnabledState(settings, metadata)` helper is the source of truth, used by both
  the upload payload (`extractProSettings`) and the success-toast counts (`isProEnabled` /
  `isDriftEnabled` / `countEnabledDisabled`), so the toast can never report a state different from
  what was uploaded.
- `extractProSettings` now writes `settings.pro.enabled = effectiveProEnabled` and, when a
  `drift_detection` block exists, `settings.pro.drift_detection.enabled = effectiveDriftEnabled`.
  The collapse runs on the JSON-sanitized copy, so the source instance is never mutated.

**Why `settings.pro.enabled` defaults to true (not the CLI's previous strict-bool default-false):**
Atmos Pro already defaults a missing/non-bool `pro.enabled` to enabled. If the CLI wrote `false` for
a component that has a `pro` block but omits `enabled` (e.g. `pro: {drift_detection: {enabled:
true}}`), it would regress default-enabled components by disabling their drift. Default-true aligns
the CLI with the Pro side; the new gating only ever turns things **off** when an outer level is
explicitly disabled.

**Disabled components are still uploaded (not omitted).** Omitting them would make Atmos Pro's
`markMissingAsOrphaned` reconciler mark them `orphaned`, which is wrong. Uploading them as
`pro.enabled: false` makes Atmos Pro show them `disabled`.

**No Atmos Pro code change is required.** `settings.pro.enabled` already maps to `instances.enabled`,
and the upsert transitions automatically on the next upload: `enabled true→false` sets `disabled_at`,
`drift_enabled true→false` sets `drift_status = "disabled"`. The stuck `error` rows self-heal — no
data migration.

## Verification

- `go test ./pkg/list/...` — new `TestExtractProSettings*` cases plus the updated toast tests pass.
- End-to-end: after this change ships, run `atmos list instances --upload` and confirm in Atmos Pro
  that the previously-stuck disabled instances flip to `enabled=false`, `drift_enabled=false`,
  `drift_status=disabled`, with `disabled_at` set.
- Confirm the next scheduled drift cycle no longer dispatches them (no new `missing_plan_result`
  runs for those stack/component pairs).

## Recommendations

- **Atmos Pro could also accept `metadata.enabled` defensively.** This fix closes the gap at the
  source (the CLI), but the ingestion contract still has no `metadata` field. Persisting it would
  make the platform robust against older CLIs that predate this fix.
- **Duplicate-repository rows are a separate issue.** A stack can end up with two rows for the same
  component (distinct `repository_id`, both `deleted_at IS NULL`). That is a distinct
  duplicate-repository question, not addressed here.
