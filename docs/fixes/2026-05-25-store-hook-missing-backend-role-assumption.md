# Fix: Store hook doesn't assume backend role when reading terraform outputs

**Date:** 2026-05-25

**Issue:** [#2433](https://github.com/cloudposse/atmos/issues/2433)
**Related:** [#2428](https://github.com/cloudposse/atmos/issues/2428) (closed, superseded by #2432 and #2433)

## Problem

When a store has `identity: dev`, the hook correctly assumes the dev
role for writing to SSM. However, reading terraform outputs requires
accessing S3 state via the backend's `assume_role` (e.g.,
`dev -> tfstate-role`). The hook doesn't chain through the backend's
`assume_role` — it only uses the store identity directly, which can't
access the state backend.

### Reproducer

```yaml
# Backend config with assume_role
terraform:
  backend:
    s3:
      assume_role:
        role_arn: arn:aws:iam::111111111111:role/tfstate-access

# Store with identity
stores:
  ssm:
    type: aws-ssm-parameter-store
    identity: dev
    options:
      region: us-east-1
      prefix: /atmos/

# Hook config
hooks:
  store-outputs:
    command: store
    events: ["after-terraform-apply"]
    name: ssm
    outputs:
      my_output: .my_output
```

```bash
atmos terraform apply my-component -s my-stack
```

Apply succeeds (Atmos handles the full credential chain), but the
after-apply hook fails:

```
✗ Fetching my_output output from my-component in my-stack
Error: failed to retrieve terraform outputs: No valid credential sources found
```

### Expected behavior

The hook should use the same backend credential chain as
`terraform apply` when reading outputs. The backend config specifies
an `assume_role`, and the hook's `terraform output` subprocess should
honor it.

### Environment

- Atmos 1.218.0
- macOS darwin/arm64
- Backend: S3 with `assume_role`
- Auth: `atmos auth` with AWS IAM Identity Center

## Root cause

The hook runs with `authManager == nil` and `SkipInit=true`, so:

1. **No auth manager for the `terraform output` subprocess.** The hook
   invokes `terraform output -json` to read the component's outputs,
   but this subprocess needs credentials to access the S3 state
   backend. Without the auth manager, the environment variables for
   the backend's `assume_role` chain are never set.

2. **Store identity ≠ backend access identity.** The store's
   `identity: dev` is correctly assumed for the SSM write, but the
   `terraform output` step needs a different credential chain:
   `dev -> tfstate-access-role` (as specified in
   `terraform.backend.s3.assume_role.role_arn`). The hook doesn't
   perform this second role assumption.

3. **The main `terraform apply` succeeds** because Atmos's terraform
   executor sets up the full credential chain (including backend role
   assumption) before invoking terraform. The hook runner does not
   replicate this setup for its `terraform output` subprocess.

### Code path to investigate

1. Hook execution in `pkg/hooks/` — the store command hook handler
   calls `terraform output` via `pkg/terraform/output/`.
2. `pkg/terraform/output/executor.go` — the output executor runs
   `terraform output -json` but may not be setting up the backend
   credential chain.
3. The credential chain for the backend's `assume_role` is normally
   set up in `internal/exec/terraform.go` before invoking terraform
   commands — this setup does not happen for the hook's output
   subprocess.
4. Compare with how the main `terraform apply` path sets up
   credentials in `internal/exec/terraform.go` and ensure the hook
   runner replicates the relevant parts.

## Fix

TBD — requires investigation into:

1. How to thread the auth manager (or at minimum the resolved
   environment variables) from the parent `terraform apply` into the
   hook's `terraform output` subprocess.
2. Whether the hook should inherit the parent command's full
   environment (which already has the correct credentials after
   the `apply` succeeded).
3. Whether `SkipInit=true` is appropriate for the output read — the
   state backend is already initialized by the parent `apply`, so
   re-init is unnecessary, but the credentials must still be present.

## Workaround

Remove `identity` from the store and run inside `atmos auth shell`:

```bash
atmos auth shell --identity dev
atmos terraform apply my-component -s my-stack
# ✓ Hook fires and succeeds because the shell session
#   has the dev role's credentials in the environment,
#   and terraform can chain to the backend role via
#   the backend config's assume_role.
```

## Tests

- Unit test: mock the terraform output executor and verify it receives
  the correct environment variables (including backend assume_role
  credentials) when invoked from a store hook.
- Integration test: verify a store hook can read terraform outputs
  from an S3 backend that requires role assumption, both with and
  without a store `identity`.

---

## Related

- [#2432](https://github.com/cloudposse/atmos/issues/2432) — Store
  hooks don't fire when stack is selected via interactive prompt
  (companion issue from the same investigation).
- [#2428](https://github.com/cloudposse/atmos/issues/2428) — Original
  consolidated issue (closed in favor of #2432 and #2433).
- [#2357](https://github.com/cloudposse/atmos/issues/2357) — Related
  auth resolver injection issue for hooks.
