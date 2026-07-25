# Fix: Restore auth-context propagation for `list`/`describe stacks` template and YAML-function evaluation

**Date:** 2026-07-24

## Summary

`list` and `describe stacks` commands that evaluate Go templates (`atmos.Component()`) or
YAML functions (`!terraform.state`/`!terraform.output`) without an explicit `--identity`
flag ran with no root `AuthContext`. A nested target with no `auth:` section of its own
inherited that empty context and the AWS SDK fell through to ambient/IMDS credentials —
silently authenticating as the wrong identity, or hard-failing where IMDS isn't available
(CI, local dev). This restores automatic default-identity resolution whenever template or
YAML-function evaluation is enabled, while keeping non-evaluating inventory runs and
`--identity=false` credential-free.

## Context

Commit `583aa66448` (PR [#2665](https://github.com/cloudposse/atmos/pull/2665),
v1.222.0-rc.14) narrowed `createAuthManagerForList` from automatic stack-scan
default-identity resolution to explicit-identity-only resolution. That change fixed a real
performance regression (every stack triggered identity resolution even for plain inventory
listing), but `list` still processes Go templates by default, so `atmos.Component()` (and,
depending on flags, `!terraform.state`/`!terraform.output`) could run with no root
`AuthContext` at all. A nested target that declares no `auth:` section of its own inherits
that empty context and the AWS SDK falls back to ambient/IMDS credentials instead of the
configured identity.

`describe stacks` had a narrower version of the same bug: its auth-manager gate checked
`ProcessYamlFunctions` only, ignoring `ProcessTemplates`, so `atmos.Component()`-only
stacks hit the same problem there too.

A related band-aid landed on 2026-07-21
(`docs/fixes/2026-07-21-terraform-lint-auth-disabled-yaml-functions.md`) that widened
`isRecoverableInWarnMode` to also treat `ErrGetObjectFromS3` (any backend read failure,
including credential refresh) as recoverable under `--error-mode=warn/silent`. That masked
this bug's symptoms for `terraform lint` specifically, but it also silently hid genuine
credential misconfiguration in every other warn/silent run. Now that root auth-context
propagation is restored, this fix reverts that broadening — credential/backend-read
failures stay fatal in every error mode again; only a genuinely unprovisioned Terraform
state/output remains eligible for the `//` default or warn/silent degradation.

## Changes

- `cmd/list/utils.go`: `createAuthManagerForList` gains `processTemplates`/
  `processYamlFunctions` parameters and creates the stack-scan `AuthManager` whenever
  either is enabled or an identity is explicitly named. Only an explicit
  `--identity=false` opts out unconditionally. The auth-manager creation call is behind
  a new `AuthManagerFactory` interface (`listAuthManagerFactory`, mockgen-generated
  `mock_utils.go`) so the policy can be tested without real authentication.
- All `list` command call sites (`components.go`, `dependencies.go`, `instances.go`,
  `metadata.go` and `stacks.go`, `settings.go` via `initConfigAndAuth`, `values.go`) now
  pass their resolved `ProcessTemplates`/`ProcessFunctions` options through.
- `cmd/list/instances.go`: `AuthDisabled` now reflects only an explicit
  `--identity=false`, not "no identity provided" — previously conflated, which
  suppressed auth even when templates/functions needed it.
- `cmd/list/sources.go`: dropped the standalone, explicit-identity-only
  `createAuthManagerForSources` in favor of the shared `createAuthManagerForList`, so
  `list sources` gets the same policy as every other list command; the resulting error
  is wrapped with `errUtils.ErrAuthenticationFailed`.
- `cmd/describe_stacks.go`: extracted `shouldCreateDescribeStacksAuthManager` and widened
  its condition to `processTemplates || processYamlFunctions || identityExplicit`
  (previously `processYamlFunctions || identityExplicit`).
- `internal/exec/template_funcs_component.go`: extracted the `tfoutput.ExecuteWithSections`
  call behind a new `ComponentFuncOutputsExecutor` interface (`componentFuncOutputsExecutor`,
  mockgen-generated `mock_template_funcs_component.go`) so a test can assert the
  `AuthContext` `atmos.Component()` passes to terraform-output without contacting AWS. No
  behavior change.
- Replaced the remaining bare-func-var test seams for auth-manager creation with the same
  interface + generated-mock pattern, following up on CodeRabbit's PR #2801 review rather
  than deferring it: `internal/exec/terraform_query.go`'s `authManagerFactory` (used by
  `ExecuteTerraformQuery`'s multi-component path) is now typed as `AuthManagerQueryFactory`,
  backed by `defaultAuthManagerQueryFactory` and a generated `mock_terraform_query.go`. All
  three auth-manager-factory seams touched by this PR now share one consistent style
  (exported one-method interface, `default*` struct delegating to the real `pkg/auth` call,
  package var of the interface type, `mockgen -source=$GOFILE` directive) instead of a mix
  of hand-rolled closures — matching the existing `TerraformOutputGetter` pattern in
  `internal/exec/terraform_output_getter.go`.
- `internal/exec/yaml_func_terraform_state.go`, `internal/exec/yaml_func_utils.go`:
  `isRecoverableInWarnMode` reverted to equal `isRecoverableTerraformError` — drops the
  `ErrGetObjectFromS3` tolerance added 2026-07-21 (see Context above).
- Tests: new `cmd/list/utils_auth_test.go`
  (`TestCreateAuthManagerForList_EvaluationPolicy`, a table test covering
  templates-only, yaml-functions-only, both disabled, explicit identity, and explicit
  `--identity=false`, now driven by the generated `MockAuthManagerFactory`);
  `cmd/describe_stacks_test.go`
  (`TestShouldCreateDescribeStacksAuthManager`); `internal/exec/template_funcs_component_test.go`
  (`TestComponentFunc_AuthlessTargetPassesParentAuthContextToTerraformOutput`, now driven by
  the generated `MockComponentFuncOutputsExecutor`, and a new `resolveComponentFuncAuthManager`
  subtest for an auth-less target); updated
  `internal/exec/describe_stacks_component_processor_test.go`,
  `internal/exec/yaml_func_terraform_state_yq_defaults_test.go`, and
  `internal/exec/yaml_func_utils_lenient_test.go` for the `ErrGetObjectFromS3` reversal;
  updated `internal/exec/terraform_query_test.go`, `internal/exec/terraform_utils_test.go`,
  and `internal/exec/terraform_all_test.go` to drive `authManagerFactory` through the
  generated `MockAuthManagerQueryFactory` instead of hand-written closures.

## Validation

```shell
go build ./...
go test ./cmd/... ./internal/exec/...
go test ./cmd/list/... -run TestCreateAuthManagerForList_EvaluationPolicy -v
go test ./cmd/... -run TestShouldCreateDescribeStacksAuthManager -v
go test ./internal/exec/... -run 'TestComponentFunc_AuthlessTargetPassesParentAuthContextToTerraformOutput|TestResolveComponentFuncAuthManager|TestIsRecoverableInWarnMode|TestProcessCustomYamlTagsLenient_S3CredentialFailure_Warn' -v
go test ./internal/exec/... -run 'TestCreateQueryAuthManager|TestExecuteTerraformQueryRoutesThroughSchedulerAdapter|TestExecuteTerraformAffectedRoutesThroughSchedulerAdapter|TestCreateQueryAuthManagerPropagatesFactoryError|TestExecuteTerraformAll_AuthManagerResolverWired|TestExecuteTerraformAll_AuthManagerCreationError' -v
```

`go build ./...` passed cleanly. The full package run and all targeted runs above passed.

## Follow-ups

None. CodeRabbit's PR #2801 review offered to file a follow-up issue to migrate
`createAuthManagerWithStackScan` and `executeComponentFuncTerraformOutputs` to a
generated-mock interface; that migration (plus the equivalent, previously
unflagged `authManagerFactory` seam in `internal/exec/terraform_query.go`) was
done directly in this PR instead, so no follow-up issue is needed.
