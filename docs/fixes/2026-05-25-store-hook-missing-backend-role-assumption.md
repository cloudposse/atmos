# Fix: Store hook doesn't assume backend role when reading terraform outputs

**Date:** 2026-05-25

**Issue:** [#2433](https://github.com/cloudposse/atmos/issues/2433)
**PR:** [#2520](https://github.com/cloudposse/atmos/pull/2520)
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
terraform:
  backend:
    s3:
      assume_role:
        role_arn: arn:aws:iam::111111111111:role/tfstate-access

stores:
  ssm:
    type: aws-ssm-parameter-store
    identity: dev
    options:
      region: us-east-1
      prefix: /atmos/

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

Apply succeeds, but the after-apply hook fails:

```text
✗ Fetching my_output output from my-component in my-stack
Error: failed to retrieve terraform outputs: No valid credential sources found
```

## Root cause

The hook runs with `authManager == nil` and `SkipInit=true`:

1. **No auth manager for the `terraform output` subprocess.** The hook
   invokes `terraform output -json` to read the component's outputs,
   but this subprocess needs credentials to access the S3 state
   backend. Without the auth manager, the environment variables for
   the backend's `assume_role` chain are never set.

2. **Store identity ≠ backend access identity.** The store's
   `identity: dev` is correctly assumed for the SSM write, but the
   `terraform output` step needs a different credential chain:
   `dev -> tfstate-access-role`. The hook doesn't perform this
   second role assumption.

3. **The main `terraform apply` succeeds** because `ExecuteTerraform`
   sets up the full credential chain (via `setupTerraformAuth` and
   `prepareComponentExecution`) before invoking terraform. But
   `ExecuteTerraform` takes `info` by value, so the populated
   `AuthContext` and `AuthManager` don't flow back to PostRunE.

## Fix

1. **Persist auth context after `prepareComponentExecution`.**
   `ExecuteTerraform` (`internal/exec/terraform.go`) now calls
   `SetLastAuthContext(info.AuthContext, info.AuthManager)` after auth
   setup completes, storing credentials in a thread-safe package-level
   cache (`internal/exec/terraform_auth_context.go`).

2. **Inject into PostRunE hook info.** `runHooksWithOutput`
   (`cmd/terraform/utils.go`) calls `GetLastAuthContext()` after
   `ProcessCommandLineArgs` creates the fresh `info`, and populates
   `info.AuthContext` and `info.AuthManager`. The hook's
   `terraform output` subprocess then has the same credential chain
   as the parent apply path.

3. **`SkipInit=true` remains correct.** The `.terraform/` directory
   is already initialized by the parent `apply`. The hook only needs
   credentials in the environment — not a re-init.

### Files changed

- `internal/exec/terraform.go` — `SetLastAuthContext` call after
  `prepareComponentExecution`.
- `internal/exec/terraform_auth_context.go` — thread-safe auth
  context cache (`SetLastAuthContext`, `GetLastAuthContext`,
  `ClearLastAuthContext`).
- `cmd/terraform/utils.go` — auth context injection in
  `runHooksWithOutput`.

## Tests

- `TestSetLastAuthContext_RoundTrips` — verifies set/get round-trip.
- `TestGetLastAuthContext_ReturnsNilWhenUnset` — nil when no auth set.
- `TestClearLastAuthContext_ResetsState` — clear resets to nil.
- `TestSetLastAuthContext_OverwritesPrevious` — latest write wins.
- `TestRunHooksWithOutput_InjectsLastAuthContext` — calls the real
  `runHooksWithOutput` from the demo-stacks fixture with a pre-set
  auth context, verifying the injection path end-to-end.

## Workaround

Remove `identity` from the store and run inside `atmos auth shell`:

```bash
atmos auth shell --identity dev
atmos terraform apply my-component -s my-stack
```

---

## Related

- [#2432](https://github.com/cloudposse/atmos/issues/2432) — Store
  hooks don't fire when stack is selected via interactive prompt.
- [#2428](https://github.com/cloudposse/atmos/issues/2428) — Original
  consolidated issue (closed in favor of #2432 and #2433).
- [#2357](https://github.com/cloudposse/atmos/issues/2357) — Related
  auth resolver injection issue for hooks.
