# `atmos list` / `atmos describe stacks` Try (and Fail) to Read Remote Terraform State

**Date:** 2026-07-28 **Severity:** High — in a multi-account repository, `atmos list
stacks --identity <one-stage>` aborts with an S3 `AccessDenied` reading *another*
stage's state bucket, making inventory commands unusable
**Issue:** https://github.com/cloudposse/atmos/issues/2566 (reported as a 1.217
regression against 1.216) **Reproducer:**
`cmd/list/inventory_credentialed_functions_test.go`

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bug fix. There is no new command, flag, or feature — inventory
listing simply stops performing remote-state reads it never needed, and an existing
`describe stacks` failure gains actionable hints. Per the repo's label decision tree
that makes it a `patch`, which does not require a `website/blog/` post or a roadmap
milestone.

## Symptom

A multi-stage repository where each stage lives in a distinct AWS account with its own
state bucket, and a `customer` component reads a var out of the stage's `global`
component:

```yaml
data_bucket_name: !terraform.state global "{{ .vars.environment }}-{{ .settings.context.instance }}-global" data_bucket_name
```

Listing the inventory with credentials for one stage tries to read every *other*
stage's bucket:

```console
$ atmos list stacks --identity prd-bastille-access
 Error: failed to read Terraform state for component `global` in stack `dev-pen`
   in YAML function: `!terraform.state global dev-pen data_bucket_name`
   operation error S3: GetObject, ... StatusCode: 403, api error AccessDenied
   (dev-pen-tfstate/global/dev-pen-global/terraform.tfstate)

$ atmos list stacks --identity dev-pen-access
 Error: ... AccessDenied (prd-bastille-tfstate/global/prd-bastille-global/terraform.tfstate)
```

Reversing the identity just reverses which bucket is unreachable. There is no identity
that works, because the scan spans every account while any one identity holds
credentials for exactly one.

## Root Cause

`skipCredentialBackedYAMLFunctionsForInventory` (`cmd/list/utils.go`) already knew that
inventory commands must not perform credentialed reads — but it applied the skip **only
when no AuthManager existed**:

```go
func skipCredentialBackedYAMLFunctionsForInventory(skip []string, authManager auth.AuthManager) []string {
	if authManager != nil {
		return skip // ← the bug
	}
	...
}
```

The guard is backwards: it stops applying the moment credentials exist. When the issue was
filed, `createAuthManagerForList` returned a non-nil manager exactly when the user passed
`--identity`, so:

- `atmos list stacks` (no identity) → manager is nil → functions skipped → works.
- `atmos list stacks --identity X` → manager is non-nil → functions evaluated across
  **all** stacks → the first stack outside X's account aborts the command.

**#2801 widened the blast radius.** `createAuthManagerForList` now also returns a manager
whenever templates or YAML functions are processed — and both default to `true`:

```go
if identityName == "" && !processTemplates && !processYamlFunctions {
	return nil, nil
}
```

So on current `main` a plain `atmos list stacks`, with no `--identity` at all, gets a
non-nil manager, skips nothing, and attempts the cross-account reads. The reported failure
no longer needs the flag to reproduce.

Note that #2801's `--error-mode=warn` (the default) does not paper over this: it limits
graceful degradation to *unprovisioned* state/output, so credential and IMDS failures stay
fatal by design.

The reasoning already recorded on `createAuthManagerForList` — *"inventory commands …
can span many stacks; implicitly resolving default identities for every stack makes
discovery fail when one unrelated … provider is offline"* — applies just as much once
credentials are present. Authenticating one account does not make the other accounts'
state readable; it only ensures the reads are attempted.

The value was never used regardless: `extract.Stacks` / `extract.UniqueComponents`
project the describe result down to stack and component **names**, so the resolved
`!terraform.state` result is computed, and then discarded.

## Fix

### 1. Inventory listing never evaluates credential-backed YAML functions

`skipCredentialBackedYAMLFunctionsForInventory` no longer takes an `authManager` and
always merges the credentialed function list (`!terraform.state`, `!terraform.output`,
`!store`, `!store.get`, `!secret`, `!aws.*`) into the caller's `--skip`. The names are
extracted into `credentialBackedYAMLFunctions()` and derived from the `u.AtmosYamlFunc*`
constants, so a rename is a compile error rather than a silent gap.

All four call sites (`list stacks`, `list components`, `list dependencies`,
`list instances`) are updated. A skipped function leaves its raw string in place, which
the name-projecting extractors ignore, so the inventory itself is unchanged.

This also removes a surprise: inventory output no longer depends on whether `--identity`
happened to be supplied.

Scope note: this covers YAML functions only. Go templates calling `atmos.Component(...)`
can still perform authenticated cross-account reads during a list — #2801 deliberately
restored auth for that path, and `--process-templates=false` remains the opt-out. Closing
that gap needs per-stack identity resolution and is out of scope here.

`initAndExtractComponents` gained a small split — `executeAndExtractComponents` — so the
describe/extract half can be driven with an injected AuthManager, mirroring
`executeAndExtractStacks` in `stacks.go`.

### 2. `describe stacks` explains repository-wide failures

Unlike listing, `describe stacks` legitimately resolves YAML functions — that is its
documented job. But an **unfiltered** `describe stacks` walks every component in every
stack, so the same multi-account topology produces a raw S3 403 with no indication of
how to proceed.

`explainRepositoryWideYAMLFunctionFailure`
(`internal/exec/describe_stacks_component_processor.go`) now enriches that error with
hints naming the failing component/stack and the three ways out:

- `atmos describe stacks --stack <stack>` (scope to a stack you hold credentials for)
- `atmos describe stacks --skip terraform.state --skip terraform.output`
- `atmos describe stacks --process-functions=false`

Two guards keep the hints from becoming wrong advice, since the `list` commands share
this processor:

- `filterByStack` must be empty. A caller who already scoped the run asked for that stack
  specifically, so the underlying error is the whole answer.
- The terraform state/output functions must not already be in `skip`. That is exactly
  what distinguishes a `describe stacks` run from a `list` run after fix (1), so a
  `list stacks` failure never suggests `atmos describe stacks --stack …`.

Enrichment is transparent to `errors.Is`, which matters because callers branch on
sentinels propagated from the inner describe (e.g. `ErrCircularDependency` from a
`!terraform.state` cycle).

## Verification

- `cmd/list/inventory_credentialed_functions_test.go` builds a two-stage repository in a
  temp directory whose components reference `!terraform.state` in a stack that does not
  exist, standing in for a cross-account bucket the caller cannot read (deterministic,
  no network, no credentials, no timeouts).
  - `TestCrossAccountFixture_FailsWhenCredentialBackedFunctionsAreEvaluated` is the
    prerequisite check: it proves the fixture really does fail when nothing is skipped,
    so the regression assertions cannot become vacuous.
  - `TestExecuteAndExtractStacks_SkipsCredentialBackedFunctionsWithIdentity` and
    `TestInitAndExtractComponents_SkipsCredentialBackedFunctionsWithIdentity` drive the
    real code paths with a mock AuthManager (the `--identity` case) and assert both
    stacks/components come back. Both **fail** on the pre-fix code with the reported
    error and pass after.
  - `TestSkipCredentialBackedYAMLFunctionsForInventory` pins the skip set, asserts the
    tokens are bare (the `!` is trimmed, matching `skipFunc`), and checks the caller's
    slice is neither mutated nor duplicated into.
- `internal/exec/describe_stacks_yaml_function_hints_test.go` covers the hint helper in
  both directions: hints present on an unfiltered scan, absent on a `--stack`-scoped run,
  absent when the state functions are already skipped (the `list` caller), nil passed
  through, and wrapped sentinels still matched by `errors.Is`.
- `go test ./cmd/list/... ./internal/exec/... ./pkg/list/...` — the only failures are
  three pre-existing ones unrelated to this change (`TestCopyFile_FailCreate`,
  `TestProcessChdirFlag`, `TestWriteEnvToFile_ErrorCases`), which fail identically on a
  clean tree because the sandbox runs as root and read-only file modes do not block
  writes.

## Recommendations

- **Consider extending the same rule to the remaining read-only inspection commands.**
  `list values` / `list vars` / `list settings` do surface resolved values, so they are
  correctly out of scope here, but they will hit the same wall in a multi-account repo.
  A per-stack identity resolution (rather than one identity for the whole scan) is the
  real long-term answer for them.
- **Skip credentialed functions centrally.** This is the third fix in this class
  (see `docs/fixes/2026-06-23-secret-list-credential-free-skip.md` and
  `docs/fixes/2026-06-22-describe-respect-metadata-enabled.md`), each maintaining its own
  skip list. Pushing the policy into the describe pipeline — "this caller only needs
  names / has no credentials" — would stop new callers from reintroducing it.
