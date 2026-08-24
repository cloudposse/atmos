# `atmos secret list -s <stack>` Fails Reading Terraform State Even When Authenticated

**Date:** 2026-06-23 **Severity:** High — on a repo whose components reference
remote state via `!terraform.state` / `!terraform.output` (or `!store`),
`atmos secret list -s <stack>` aborts with an AWS credentials error even
immediately after a successful `atmos auth login`, because credential-free
enumeration disables auth but still evaluates the credentialed function
**Issue:** https://github.com/cloudposse/atmos/issues/2639 (the `secret list`
credential-free enumeration path was introduced by #2646; this corrects a
regression in that path) **Reproducer:** `cmd/secret/shared_test.go`
(`TestCredentialFreeSkip`), plus the existing `cmd/secret` enumeration/list
tests

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bug fix in the `cmd/secret` listing path: it makes
credential-free secret listing genuinely credential-free by skipping the YAML
functions that perform authenticated backend reads. There is no new command,
flag, or feature — only a correction so `secret list` stops attempting
remote-state reads it never needs. Per the repo's label decision tree that makes
it a `patch`, which does not require a `website/blog/` post or a roadmap
milestone.

## Symptom

Immediately after authenticating the stack's identity:

```console
$ atmos auth login -i <stack-default-identity>
✓ Authentication successful!

$ atmos secret list --stack=<stack>
 WARN  Failed to read Terraform state after all retries exhausted \
   file=<component>/<workspace>/terraform.tfstate bucket=<central-tfstate-bucket> attempts=3 \
   error="operation error S3: GetObject, ... get credentials: failed to refresh cached credentials, \
   operation error STS: AssumeRole, ... no EC2 IMDS role found, ... \
   Get \"http://169.254.169.254/latest/meta-data/iam/security-credentials/\": \
   dial tcp 169.254.169.254:80: connect: no route to host"

 Error: failed to read Terraform state for component `<component>` in stack `<stack>`
   in YAML function: `!terraform.state <component> .<output>`
```

Listing only needs the **static** secret declarations, so it should never read
remote state at all. The same failure occurs for
`atmos secret list -s <stack> -c <component>` without `--verify`.

## Root Cause

`secret list` is designed to be credential-free when it isn't fully scoped with
`--verify`:

- `enumerateSecretScopes` (`cmd/secret/enumerate.go`) resolves all stacks via
  `ExecuteDescribeStacksWithAuthDisabled(...)`.
- `loadServiceForList` (`cmd/secret/shared.go`) resolves a single component with
  `AuthDisabled: true`.

Both disabled authentication (the #2646 fix for #2639, so a large stack doesn't
run one full auth cycle per component) **but left YAML-function processing on
with a skip list of only `["secret"]`.** So a component's `!terraform.state` /
`!terraform.output` / `!store` references in `vars` / `settings` were still
evaluated. With auth disabled:

1. `GetTerraformState` (`internal/exec/terraform_state_utils.go`) sees
   `AuthDisabled`, skips per-component auth resolution, and reads the backend
   with a **nil** auth context.
2. `LoadConfigWithAuthAndEnv` (`pkg/aws/identity/identity.go`) logs *"Using
   standard AWS SDK credential resolution (no auth context provided)"* and falls
   back to the default chain.
3. The S3 backend assumes its configured `role_arn` with no base credentials, so
   the SDK ultimately dials the EC2 IMDS endpoint — unreachable on a workstation
   → the error above.

Before #2646, `secret list` authenticated per component, so these reads had
credentials (slow but working). #2646 removed the authentication without
removing the reads, turning a slow path into a hard failure for any repo whose
component `vars` reference remote state.

`secrets.ExtractDeclarations` (`pkg/secrets/registry.go`) only reads the static
`secrets.vars` section (name, scope, backend type/name, description) — it never
needs a resolved `!terraform.state` or `!store` value, so evaluating those
functions during listing was pure, failure-prone overhead.

## Fix

Add `credentialFreeSkip()` (`cmd/secret/shared.go`) listing every credentialed
read function and use it in both credential-free paths:

```go
func credentialFreeSkip() []string {
    return []string{
        strings.TrimPrefix(u.AtmosYamlFuncSecret, "!"),          // !secret
        strings.TrimPrefix(u.AtmosYamlFuncStore, "!"),           // !store
        strings.TrimPrefix(u.AtmosYamlFuncStoreGet, "!"),        // !store.get
        strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!"), // !terraform.output
        strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!"),  // !terraform.state
    }
}
```

- `enumerateSecretScopes` and `loadServiceForList` (non-`--verify`) now pass
  `credentialFreeSkip()` instead of `["secret"]`. A skipped function leaves its
  raw string in place, which `ExtractDeclarations` ignores, so declaration
  discovery is unchanged.
- The **authenticated** paths are untouched: `loadServiceAndConfig` (used by
  `get` / `set` / `exec` / `shell` and `secret list --verify`) still passes only
  `["secret"]`, because it has real credentials and may legitimately resolve
  `!terraform.state` / `!store` for the component environment.

## Verification

- `go test ./cmd/secret/...` — `TestCredentialFreeSkip` pins the skip set and
  asserts the tokens are bare (the `!` is trimmed, matching `skipFunc`); it
  references the `u.AtmosYamlFunc*` constants so a rename is a compile error,
  not a silent gap. Existing enumeration/list tests still pass.
- End-to-end on a representative multi-account repo whose components reference
  cross-account `!terraform.state`, running the **identical** command with only
  the atmos binary varying:
  - **Before:** `atmos secret list -s <stack>` aborted with the assume-role →
    IMDS error above, even immediately after `atmos auth login` for that stack's
    identity.
  - **With this fix:** completes (`rc=0`) with **zero** state reads and **zero**
    credential-resolution fallbacks in debug logs, reporting the declared
    secrets (or "No secrets declared in stack "<stack>"" when none are
    declared).

## Recommendations

- **Skip credentialed functions centrally when auth is disabled.** Rather than
  relying on each credential-free caller to maintain its own skip list, the
  describe pipeline could skip `!terraform.*` / `!store` whenever `authDisabled`
  is set, so a future credential-free caller can't reintroduce this class of
  failure. Tracked separately.
- **Surface a clearer error.** When a credentialed function is evaluated with
  auth disabled, the failure currently surfaces as a low-level AWS IMDS error. A
  targeted message ("`!terraform.state` requires credentials but authentication
  is disabled") would be far easier to diagnose.
