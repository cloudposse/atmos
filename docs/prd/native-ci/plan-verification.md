# Native CI Integration - Plan Verification

> Related: [Planfile Storage](./planfile-storage.md) | [CI Detection](./ci-detection.md)

## FR-6: Plan Verification

**Requirement**: Verify downloaded planfile matches current state before apply.

**Behavior**:
- Compare downloaded plan against fresh plan output
- Use semantic comparison (not byte-for-byte)
- Fail apply if plan has drifted
- Support configuration via `--verify-plan` flag

**Validation**:
- Detects resource changes between plan and apply
- Provides clear error message on verification failure
- Suggests re-running plan when drift detected

## Key Design Decision: Use Existing Plan-Diff

The `--verify-plan` flag uses the existing plan-diff implementation (`internal/exec/terraform_plan_diff*.go`) for semantic comparison:

```bash
# Download planfile and verify it matches current plan
atmos terraform apply vpc -s plat-ue2-dev --download-planfile --verify-plan
```
