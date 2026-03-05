# Native CI Integration - Hooks Integration

> Related: [CI Detection](./ci-detection.md) | [Implementation Status](./implementation-status.md) | [Overview](../overview.md)

## Phase 3: CI Hook Commands

CI behaviors integrate at existing hook points in `pkg/hooks/`:

```go
BeforeTerraformInit  = "before.terraform.init"   // Download planfiles here
AfterTerraformPlan   = "after.terraform.plan"    // Upload planfiles, PR comment, job summary
AfterTerraformApply  = "after.terraform.apply"   // Update PR comment, job summary, export outputs
```

## Hook Commands to Create

| File | Purpose | Status |
|------|---------|--------|
| `pkg/hooks/ci_upload.go` | CI upload hook command | Phase 3 |
| `pkg/hooks/ci_download.go` | CI download hook command | Phase 3 |
| `pkg/hooks/ci_comment.go` | CI comment hook command | Phase 3 |
| `pkg/hooks/ci_summary.go` | CI summary hook command | Phase 3 |
| `pkg/hooks/ci_output.go` | CI output hook command | Phase 3 |

## Integration Points

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/hooks/hooks.go` | Register new CI hook commands |
| `internal/exec/terraform.go` | Integrate CI hooks at lifecycle points |

### Hook Lifecycle

1. **Before Init**: Download planfiles from storage (for apply workflows)
2. **After Plan**: Upload planfiles, write job summary, post PR comment, export CI outputs
3. **After Apply**: Update PR comment, write job summary, export terraform outputs

This keeps CI behaviors modular and allows users to extend or replace them via configuration.
