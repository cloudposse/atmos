# Native CI Integration - Plan Verification

> Related: [Planfile Storage](./planfile-storage.md) | [CI Detection](../framework/ci-detection.md)

## FR-6: Plan Verification

**Requirement**: Verify downloaded planfile matches current state before apply.

**Behavior**:
- Download stored planfile to a temp path
- Generate a fresh `terraform plan`
- Compare the two planfiles using plan-diff (`internal/exec/terraform_plan_diff*.go`)
- Fail apply if plan has drifted
- Opt-in via `--verify-plan` flag (not mandatory, not auto-enabled in CI)

**Validation**:
- Detects resource changes between stored plan and current state
- Provides clear error message on verification failure
- Suggests re-running plan when drift detected

## Verification Workflow

When `--verify-plan` is specified during apply:

1. **Download** the stored planfile from planfile storage to a **temp file** (not the normal planfile path)
2. **Show stored plan** — run `terraform show stored.tfplan` to produce text output A
3. **Run `terraform plan`** to generate a **fresh planfile** at the normal path
4. **Show fresh plan** — run `terraform show fresh.tfplan` to produce text output B
5. **Compare** text A vs text B using plan-diff semantic comparison
6. If differences detected → **fail** with a clear error showing what drifted
7. If no differences → **proceed** with apply using the fresh planfile

```bash
# Verify downloaded planfile matches current plan (download is automatic in CI mode)
atmos terraform apply vpc -s plat-ue2-dev --ci --verify-plan
```

## Performance

Verification adds one full `terraform plan` execution (~30-60 seconds). This is acceptable because:
- Verification is opt-in (not auto-enabled)
- Apply operations are already slow
- Safety matters more than speed — prevents applying stale plans

## Plan-Diff

The plan-diff implementation exists and is fully implemented:
- `internal/exec/terraform_plan_diff.go` — Main entry point
- `internal/exec/terraform_plan_diff_comparison.go` — Comparison logic
- `internal/exec/terraform_plan_diff_preparation.go` — Output normalization

The comparison is **text-based semantic comparison** — it works on `terraform show` output (not binary `.tfplan` files), normalizes plan output (strips timestamps, run IDs, ordering noise) and compares resource blocks structurally. Both the stored and fresh planfiles must be converted to text via `terraform show` before comparison.

## Integration Point

Verification runs during the `before.terraform.apply` event, after planfile download but before the actual apply. If verification fails, the apply command exits with an error before terraform is invoked.
