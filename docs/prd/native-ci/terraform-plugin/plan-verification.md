# Native CI Integration - Plan Verification

> Related: [Planfile Storage](./planfile-storage.md) | [CI Detection](../framework/ci-detection.md)

## FR-6: Plan Verification

**Requirement**: Verify downloaded planfile matches current state before deploy.

**Command**: `atmos terraform deploy` (NOT `atmos terraform apply`)

**Behavior**:
- Download stored planfile to a `stored.*` prefix path
- Generate a fresh `terraform plan` (without CI side effects — no upload, no checks, no summaries)
- Compare the two planfiles using plan-diff (`internal/exec/terraform_plan_diff*.go`)
- On drift: **fail** the deploy (mode `fail`) or **warn and proceed** (mode `warn`)
- Apply the **fresh** planfile if plans match (reconcile, don't replay): the freshly generated plan is
  built with the apply-time state and credentials, so the deploy avoids the brittleness of a saved plan —
  which goes stale when state moves, and whose base credentials come from the apply environment, not the
  plan (so a plan built on a PR can fail to apply on merge) — while the diff preserves the review
  guarantee. (Replaying the stored binary directly is still available via `--from-plan` / `--planfile`.)
- **Automatic under CI when planfile storage is configured.** Configurable via
  `components.terraform.planfiles.verify` (`fail` | `warn` | `off`, default `fail` under CI), and
  overridable per-run with `--verify-plan` (force `fail`) / `--verify-plan=false` (force `off`). Precedence:
  CLI flag > config > CI default.

**Why `deploy`, not `apply`?**
- **It's where the fresh plan exists.** Drift detection requires comparing the stored planfile against a
  *current* plan. `deploy` already re-runs `terraform plan` (then applies), so the fresh plan to diff
  against is produced as part of the command. `apply` does **not** re-plan — it applies an existing plan
  or config directly — so there is nothing to diff a stored plan against. Verification therefore belongs
  on `deploy` by construction.
- `apply` is a thin wrapper around `terraform apply` — it should not interact with planfile storage.
- `deploy` is the opinionated CI command designed for automation; clean separation keeps `apply` simple.

**Validation**:
- Detects resource changes between stored plan and current state
- Provides clear error message on verification failure
- Suggests re-running plan when drift detected

## Verification Workflow

When `atmos terraform deploy` runs in CI mode:

1. **Download** the stored planfile from planfile storage to a `stored.*` prefix path (e.g., `stored.plan.tfplan`)
2. **Run `terraform plan`** to generate a **fresh planfile** at the canonical path — this runs without any CI hooks (no upload, no status checks, no summaries)
3. **JSON of stored plan** — run `terraform show -json stored.plan.tfplan` (output A)
4. **JSON of fresh plan** — run `terraform show -json plan.tfplan` (output B)
5. **Compare** A vs B using the JSON-structural plan-diff (parsed to maps, sorted, deep-compared)
6. If differences detected → **fail** (mode `fail`) with a clear error showing what drifted, or **warn and proceed** (mode `warn`)
7. If no differences → **proceed** with apply using the fresh planfile

> **Runtime token in GitHub Actions:** the automatic download uses the GitHub Artifacts runtime API
> (when the store is `github/artifacts`), so the workflow must surface `ACTIONS_RUNTIME_TOKEN` /
> `ACTIONS_RESULTS_URL` via the [`github-runtime`](https://github.com/cloudposse/atmos/tree/main/actions/github-runtime)
> action before `deploy` — the same requirement as upload.

```bash
# Deploy downloads stored plan, verifies, and applies
atmos terraform deploy vpc -s plat-ue2-dev --ci
```

## Command Responsibility

| Command | CI Planfile Download | Plan Verification | CI Summaries/Checks/Outputs |
|---------|---------------------|-------------------|----------------------------|
| `atmos terraform plan` | N/A | N/A | Yes (upload planfile, write summary, checks, outputs) |
| `atmos terraform apply` | **No** | **No** | Yes (write summary, checks, outputs) |
| `atmos terraform deploy` | **Yes** | **Yes** | Yes (write summary, checks, outputs) |

## Performance

Verification adds one full `terraform plan` execution (~30-60 seconds). This is acceptable because:
- Deploy operations are already slow
- Safety matters more than speed — prevents applying stale plans
- The plan runs without CI overhead (no upload, no checks)

## Plan-Diff

The plan-diff implementation exists and is fully implemented:
- `internal/exec/terraform_plan_diff.go` — Main entry point
- `internal/exec/terraform_plan_diff_comparison.go` — Comparison logic
- `internal/exec/terraform_plan_diff_preparation.go` — Output normalization

The comparison is **JSON-structural** — it runs `terraform show -json` on the stored and fresh
planfiles, parses each to a map, sorts keys for determinism, and deep-compares variables, managed
resources (data sources skipped), and outputs (`internal/exec/terraform_verify_plan.go` →
`generatePlanDiff`). Sensitive values are masked and computed-hash attributes are ignored.

**Why not a naive diff?** A plan legitimately contains values that vary between review and apply —
attributes "known after apply," computed fields, hashes, ordering, timestamps. A byte-for-byte
comparison of planfiles (or of raw `terraform show` text) flags all of that as drift, so a still-valid
plan is rejected; Terraform's own saved-plan apply is stricter still (any state-lineage movement
invalidates the saved plan). Real-world verification needs **wiggle room**: tolerate benign variation
while catching substantive drift (a resource added/removed/changed). The semantic, normalized
comparison above provides exactly that — which is what makes plan-then-deploy practical rather than
perpetually failing.

## Integration Point

Verification runs inside `deploy`'s `RunE`, after the stored planfile is downloaded in `PreRunE` (via `before.terraform.deploy` CI hook). The fresh plan is generated internally (not through the plan command's CI hooks). If verification fails, deploy exits with an error before terraform apply is invoked.
