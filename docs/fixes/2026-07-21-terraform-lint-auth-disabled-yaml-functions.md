# Fix: `atmos terraform lint --identity=false` still hit AWS unauthenticated

**Date:** 2026-07-21

## Problem

CI's `[lint] quick-start-advanced` job ran:

```shell
atmos terraform lint sns-topic -s plat-ue2-dev --identity=false
```

with a comment explaining the intent: "Lint reads static HCL only. Disable the quick-start
stack's emulator identity because this CI job intentionally does not start the emulator." The
job failed anyway:

```
WARN Failed to read Terraform state after all retries exhausted file=kms-key/plat-ue2-dev/terraform.tfstate ...
error="operation error S3: GetObject, get identity: get credentials: failed to refresh cached
credentials, no EC2 IMDS role found, ..."

Error: failed to execute describe stacks: failed to read Terraform state for component kms-key
in stack plat-ue2-dev in YAML function: !terraform.state kms-key .key_arn // "arn:aws:kms:...
:000000000000" failed to get object from S3: ...
```

`sns-topic`'s stack config resolves `kms_key_arn` via
`!terraform.state kms-key .key_arn // "<fallback>"` — the `//` is meant to fall back to the
literal default when the lookup can't succeed.

## Root cause

Two things had to both be true:

1. `--identity=false` (`AuthDisabled`) only skips building an Atmos `AuthManager` — it does
   **not** turn off YAML-function evaluation. `pkg/scanners/tflint/command.go`'s `execute()`/
   `executeAffected()` still passed `info.ProcessFunctions`/`options.ProcessYamlFunctions`
   through unchanged, so `!terraform.state`/`!terraform.output` calls still ran, falling through
   to the AWS SDK's ambient/default credential chain (EC2 IMDS) with no explicit `AuthContext` to
   short-circuit them.
2. The `//` fallback (`internal/exec/yaml_func_terraform_state.go`'s `isRecoverableTerraformError`)
   only fires for `ErrTerraformStateNotProvisioned`/`ErrTerraformOutputNotFound` — i.e. "the
   backend/key genuinely doesn't exist yet" (the normal cold-start case against the example's
   `local-aws` emulator). A credential-refresh failure surfaces as `ErrGetObjectFromS3` instead,
   which isn't in that recoverable set, so it propagates as a hard error even though a `//`
   default is present.

`pkg/hooks/hooks.go` already documents and follows the correct pattern for this exact situation
(`GetHooks` forces `ProcessYamlFunctions: false` because it runs pre-auth), but nothing tied
`atmos terraform lint`'s auth-disabled path to the same rule.

## Fix

`pkg/scanners/tflint/command.go`: when auth is disabled (`--identity=false`/`AuthDisabled`),
`execute()` now forces `info.ProcessFunctions = false`, and `executeAffected()` forces both
`info.ProcessFunctions` and `options.ProcessYamlFunctions` to `false`. Lint only needs static
HCL/backend config to run TFLint — it never needed the resolved remote values — so skipping YAML
functions entirely when there's no `AuthManager` to safely reach a real backend with matches the
job's own stated intent ("lint reads static HCL only") instead of silently depending on ambient
AWS credentials being absent-but-harmless.

Not changed: `isRecoverableTerraformError`'s classification. Widening it to also treat
credential/auth failures as fallback-eligible would be a broader behavior change (silently
swallowing real credential misconfiguration in non-lint, non-auth-disabled runs too) and needs
separate product judgment — out of scope here since the auth-disabled fix above removes the only
path that reaches this case in practice.

## Tests

```shell
go test ./pkg/scanners/tflint/... -run 'TestExecuteDisablesComponentAuthDuringStackDiscovery|TestExecuteAffectedFiltersAndDeduplicatesTargets' -v
```

Both tests now assert `ProcessFunctions`/`ProcessYamlFunctions` are forced to `false` whenever
auth is disabled.
