# Native CI Integration - Plan Verification

> Related: [Planfile Storage](./planfile-storage.md) | [CI Detection](../framework/ci-detection.md)

## FR-6: Plan Verification

**Requirement**: Verify downloaded planfile matches current state before deploy.

**Command**: `atmos terraform deploy` (NOT `atmos terraform apply`)

**Behavior**:
- Download stored planfile to a `stored.*` prefix path
- Generate a fresh `terraform plan` (without CI side effects — no upload, no checks, no summaries)
- Compare the two planfiles using plan-diff (`internal/exec/terraform_plan_diff*.go`)
- Fail deploy if plan has drifted
- Apply the fresh planfile if plans match
- Enabled by default in CI mode (`--ci`)

**Why `deploy`, not `apply`?**
- `apply` is a thin wrapper around `terraform apply` — it should not interact with planfile storage
- `deploy` is the opinionated CI command designed for automation
- Clean separation: `apply` = simple, `deploy` = full CI workflow

**Validation**:
- Detects resource changes between stored plan and current state
- Provides clear error message on verification failure
- Suggests re-running plan when drift detected

## Verification Workflow

When `atmos terraform deploy` runs in CI mode:

1. **Download** the stored planfile from planfile storage to a `stored.*` prefix path (e.g., `stored.plan.tfplan`)
2. **Run `terraform plan`** to generate a **fresh planfile** at the canonical path — this runs without any CI hooks (no upload, no status checks, no summaries)
3. **Show stored plan** — run `terraform show stored.plan.tfplan` to produce text output A
4. **Show fresh plan** — run `terraform show plan.tfplan` to produce text output B
5. **Compare** text A vs text B using plan-diff semantic comparison
6. If differences detected → **fail** with a clear error showing what drifted
7. If no differences → **proceed** with apply using the fresh planfile

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

The comparison is **text-based semantic comparison** — it works on `terraform show` output (not binary `.tfplan` files), normalizes plan output (strips timestamps, run IDs, ordering noise) and compares resource blocks structurally.

## Integration Point

Verification runs inside `deploy`'s `RunE`, after the stored planfile is downloaded in `PreRunE` (via `before.terraform.deploy` CI hook). The fresh plan is generated internally (not through the plan command's CI hooks). If verification fails, deploy exits with an error before terraform apply is invoked.
