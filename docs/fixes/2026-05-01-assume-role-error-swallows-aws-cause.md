# Fix: AWS assume-role / web-identity / assume-root errors swallow the underlying STS cause

**Date:** 2026-05-01

**Issue:**

- `pkg/auth/identities/aws/assume_role.go:185-198` —
  `assumeRoleIdentity.Authenticate()` (standard `AssumeRole` path)
  builds an enriched `errUtils.ErrAuthenticationFailed` and returns
  it without chaining the underlying `err` returned by the STS SDK.
- `pkg/auth/identities/aws/assume_role.go:266-279` —
  `assumeRoleIdentity.assumeRoleWithWebIdentity()`
  (`AssumeRoleWithWebIdentity` path) does the same.
- `pkg/auth/identities/aws/assume_root.go` —
  `assumeRootIdentity.Authenticate()` (`AssumeRoot` path) does the
  same.

In all three sites the AWS SDK error (which carries the actual STS
reason — `AccessDenied`, `InvalidIdentityToken`,
`ExpiredTokenException`, `MalformedPolicyDocumentException`, etc.)
is dropped on the floor. The user sees only:

```text
Error: authentication failed: identity=<name> step=<n>: authentication failed
```

The error message is technically correct (something failed at step
`n`) but useless for debugging — the operator can't tell whether the
role doesn't exist, the trust policy rejected the principal, the
session policy is malformed, or STS throttled the request.

The chain wrapper at `pkg/auth/manager_chain.go:570` would have
surfaced the SDK error if the leaf call site had preserved it:

```go
return nil, fmt.Errorf("%w: identity=%s step=%d: %w", errUtils.ErrAuthenticationFailed, identityStep, i, err)
```

— the trailing `%w: %w` is ready to print whatever the leaf returned.
The leaf returned a sentinel with no cause attached, so the chain
formatter prints the sentinel's text twice and stops.

The error builder already exposes a `WithCause(err)` helper for
exactly this situation (`errors/builder.go:104-167`). It chains the
SDK error into the result, lets `errors.Is(result,
ErrAuthenticationFailed)` keep matching the sentinel, and extracts
any hints / safe details the cause already carried. Other auth
sites in the same package use it correctly — see
`pkg/auth/identities/aws/webflow_token.go:88-97` for the canonical
pattern.

## Status

**Fixed.** All three error builders in the AWS assume-role family
now chain the SDK error via `WithCause(err)`. The hint and context
metadata stays the same; the user-facing message gains the AWS-side
reason.

Before:

```text
Error: authentication failed: identity=my-role step=1: authentication failed
```

After:

```text
Error: authentication failed: identity=my-role step=1: authentication failed:
  operation error STS: AssumeRoleWithWebIdentity, https response error
  StatusCode: 403, RequestID: <id>, api error AccessDenied: Not authorized
  to perform sts:AssumeRoleWithWebIdentity
```

### Progress checklist

- [x] Reproduce: existing `TestAssumeRoleIdentity_Authenticate_ErrorCases`
  exercises the validation error path; added a sibling case that
  drives a real STS failure through a stub client and asserts the
  returned error chains through to the SDK message via
  `errors.Is(err, &smithy.GenericAPIError{Code: "AccessDenied"})`
  (or simpler — `assert.ErrorContains(err, "AccessDenied: <reason>")`).
- [x] Apply fix: insert `WithCause(err)` as the first builder call
  in all three error sites.
- [x] Confirm: new test passes; the existing
  `errors.Is(err, errUtils.ErrAuthenticationFailed)` assertions
  continue to pass (the sentinel is preserved by `WithCause`).
- [x] Regression: `go test ./pkg/auth/... -count=1` green;
  `go vet ./pkg/auth/...` clean; `go build ./...` succeeds.

---

## Problem

### Reproducer

```yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1
      spec:
        audience: sts.amazonaws.com

  identities:
    my-role:
      kind: aws/assume-role
      via:
        provider: github-oidc
      principal:
        # Trust policy intentionally does not authorize this caller.
        assume_role: arn:aws:iam::111111111111:role/role-with-mismatched-trust
```

Run from a context where OIDC issuance succeeds but the trust
policy on `role-with-mismatched-trust` doesn't accept the issued
token's `sub` claim:

```bash
atmos auth login --identity my-role
```

Pre-fix output:

```text
INFO Starting GitHub OIDC authentication provider=github-oidc
INFO GitHub OIDC authentication successful provider=github-oidc

Authenticate Credential Chain
Error: authentication failed: failed to authenticate via credential chain
for identity "my-role": authentication failed: identity=my-role step=1:
authentication failed
```

The error tells the operator that step 1 of the chain (the assume
itself) failed, but provides no information about *why* AWS
rejected the request. They cannot distinguish between:

- The role doesn't exist (`NoSuchEntity` / `404`).
- The trust policy doesn't authorize the principal (`AccessDenied`).
- The OIDC token is malformed or expired (`InvalidIdentityToken` /
  `ExpiredTokenException`).
- The session policy is invalid
  (`MalformedPolicyDocumentException`).
- STS is throttling (`Throttling` / `429`).

Each of those has a different remediation. The user has to enable
debug logging (`ATMOS_LOGS_LEVEL=Debug`) and re-run to recover the
information that should have been in the original error.

### Code path

1. `manager.Authenticate(ctx, "my-role")` builds chain
   `[github-oidc, my-role]`.
2. `authenticateIdentityChain` (`pkg/auth/manager_chain.go:548`)
   walks the chain. At step 1 it calls
   `assumeRoleIdentity.Authenticate(ctx, oidcCreds)`.
3. `Authenticate()` at `pkg/auth/identities/aws/assume_role.go:158-160`
   detects OIDC credentials and routes to
   `assumeRoleWithWebIdentity()`.
4. `assumeRoleWithWebIdentity()` calls
   `stsClient.AssumeRoleWithWebIdentity(ctx, input)` at line 266.
   The SDK returns an `err` carrying the AWS-side reason
   (`*smithy.GenericAPIError` for known codes,
   `*types.ResponseError` for transport, etc.).
5. The error site at lines 267-279 wraps a sentinel with
   `errUtils.Build(...).WithExplanation(...).WithHint(...).Err()`
   but never chains `err`:

   ```go
   result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
   if err != nil {
       return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
           WithExplanationf("Failed to assume IAM role '%s' using web identity (OIDC)", i.roleArn).
           WithHint("Verify the role ARN is correct in your atmos.yaml configuration").
           WithHint("Ensure the OIDC token is valid and not expired").
           WithHint("Check that the role's trust policy allows the OIDC provider").
           WithHint("For GitHub Actions OIDC, verify the repository and workflow are authorized").
           WithContext("identity", i.name).
           WithContext("role_arn", i.roleArn).
           WithContext("region", i.region).
           WithExitCode(1).
           Err()
   }
   ```

   The `err` from the SDK call is now unreachable.
6. The chain wrapper at `manager_chain.go:570` formats:

   ```go
   return nil, fmt.Errorf("%w: identity=%s step=%d: %w", errUtils.ErrAuthenticationFailed, identityStep, i, err)
   ```

   The trailing `%w` would print the SDK message if it had survived
   step 5. It didn't, so the operator sees only
   `"authentication failed: identity=my-role step=1: authentication failed"`.

The same anti-pattern exists in two more sites in the same family:

- `assume_role.go:185-197` — standard `AssumeRole` (used when the
  base credentials are AWS, not OIDC).
- `assume_root.go` — `AssumeRoot` (centralized root access via
  Identity Center permission set).

### Why the sentinel-only error is wrong here

`errUtils.Build()` is the right approach when the failure is
self-contained — config validation, missing-required-field,
malformed-input. In those cases there is no upstream `err` and the
hints + explanation are the entire story.

The STS SDK calls are different. They surface a remote service's
diagnostic, and the diagnostic is exactly the information the
operator needs to fix the problem. The hint list ("verify the role
ARN", "check the trust policy", "ensure the token is valid")
enumerates *every plausible cause*; without the SDK error, the
operator has to investigate all of them. With the SDK error, the
right hint is implicit.

### Why no test caught this

Existing `TestAssumeRoleIdentity_Authenticate_ErrorCases` and
`_ValidationErrors` (lines 532-628) exercise only paths that fail
before any STS call — nil credentials, missing principal config,
wrong credentials type. None of them drive the SDK error path,
because doing so requires a fake/stub STS client. Adding that
client and asserting on the chained cause is part of this fix.

## Fix

### Where

- `pkg/auth/identities/aws/assume_role.go` —
  `assumeRoleIdentity.Authenticate()` (around line 187),
  `assumeRoleIdentity.assumeRoleWithWebIdentity()` (around line 268).
- `pkg/auth/identities/aws/assume_root.go` —
  `assumeRootIdentity.Authenticate()`.

### What

Insert `WithCause(err)` as the first builder call in each error
site, immediately after `errUtils.Build(errUtils.ErrAuthenticationFailed)`:

```go
// AssumeRoleWithWebIdentity (assume_role.go)
result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
if err != nil {
return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
WithCause(err). // ← new
WithExplanationf("Failed to assume IAM role '%s' using web identity (OIDC)", i.roleArn).
WithHint("Verify the role ARN is correct in your atmos.yaml configuration").
WithHint("Ensure the OIDC token is valid and not expired").
WithHint("Check that the role's trust policy allows the OIDC provider").
WithHint("For GitHub Actions OIDC, verify the repository and workflow are authorized").
WithContext("identity", i.name).
WithContext("role_arn", i.roleArn).
WithContext("region", i.region).
WithExitCode(1).
Err()
}
```

`WithCause` (`errors/builder.go:122-144`):

- Chains the cause into the result via `fmt.Errorf("%w: %w",
  sentinel, cause)` — so both `errors.Is(err, sentinel)` and
  `errors.Is(err, cause)` continue to match.
- Extracts hints from the cause (`errors.GetAllHints(cause)`) and
  appends them to the builder's hint list — preserves any
  AWS-side hints already present.
- Extracts safe details from the cause and merges them into the
  builder's context map without overwriting existing keys.
- No-op if `cause == nil` — safe to call unconditionally.

### Why not use `fmt.Errorf("%w: %w", ...)` directly

The same effect could be achieved with a manual
`fmt.Errorf("%w: %w", errUtils.ErrAuthenticationFailed, err)`, but
that would lose the explanation, hints, context, and exit code
attached by the builder. `WithCause` is the path that keeps every
metadata field the builder already accumulated *and* threads the
cause into the chain.

### Why this is the correct layer

The leaf call site is the only place that has both the sentinel
context (this is an authentication failure) and the SDK error
(this is *why* it failed). Wrapping at the chain level (manager) is
too late — the manager wrapper at line 570 is generic and runs for
every step regardless of identity kind; it can't add identity-kind
specific hints. Wrapping at the caller (the user of
`manager.Authenticate`) is also too late — by the time the error
reaches them, multiple chain steps may have run and the SDK error
would have to be plumbed through several wrappers anyway.

### Why the standard library's `errors.Is` matters here

`WithCause` deliberately uses `fmt.Errorf("%w: %w", ...)` rather
than the cockroachdb `errors.Wrap` helper precisely so that
`errors.Is` from the standard library — which is what `testify`'s
`assert.ErrorIs` and most user code call — can still find the
sentinel. The double-`%w` syntax (Go 1.20+) attaches both errors to
the result's `Unwrap() []error` method, so both are reachable. The
existing `TestErrorBuilder_WithCause` (`errors/builder_test.go:420`)
verifies this property; the new tests added in this fix lean on it.

## Tests

### Unit — error path on `AssumeRoleWithWebIdentity`

`pkg/auth/identities/aws/assume_role_test.go::TestAssumeRoleIdentity_Authenticate_OIDC_ChainsSDKError`

1. Construct an `assumeRoleIdentity` with a stub STS client whose
   `AssumeRoleWithWebIdentity` returns
   `&smithy.GenericAPIError{Code: "AccessDenied", Message: "Not
   authorized to perform sts:AssumeRoleWithWebIdentity on resource
   arn:aws:iam::111111111111:role/role-with-mismatched-trust"}`.
2. Call `identity.Authenticate(ctx, oidcCreds)`.
3. Assert:
  - `errors.Is(err, errUtils.ErrAuthenticationFailed)` — sentinel
    preserved.
  - `assert.ErrorContains(err, "AccessDenied")` — SDK code threaded
    through.
  - `assert.ErrorContains(err, "Not authorized to perform sts:AssumeRoleWithWebIdentity")`
    — SDK message threaded through.

### Unit — error path on standard `AssumeRole`

`pkg/auth/identities/aws/assume_role_test.go::TestAssumeRoleIdentity_Authenticate_StandardAssume_ChainsSDKError`

Same structure but the stub returns `&smithy.GenericAPIError{Code:
"NoSuchEntity", Message: "Role with name X does not exist"}` and
the test drives the AWS-credentials branch (line 164-200).

### Unit — error path on `AssumeRoot`

`pkg/auth/identities/aws/assume_root_test.go::TestAssumeRootIdentity_Authenticate_ChainsSDKError`

Mirrors the above against `AssumeRoot`.

### Regression — existing assertions

The existing tests assert `assert.Contains(t, err.Error(), "invalid
identity config")` and similar — unchanged, those error sites use
plain sentinels (`errUtils.ErrInvalidIdentityConfig`) that don't go
through the SDK and aren't affected by this fix.

---

## Related

- `errors/builder.go:104-167` — `WithCause` / `WithCausef`
  implementation; the fix uses `WithCause(err)` exclusively because
  the error already exists as a Go `error` value.
- `errors/builder_test.go:420` — `TestErrorBuilder_WithCause`
  validates the `errors.Is` semantics that the new tests rely on.
- `pkg/auth/identities/aws/webflow_token.go:88-97` — canonical
  example of the correct pattern (HTTP transport error wrapped with
  `WithCause(err)` plus enrichment).
- `pkg/auth/manager_chain.go:548-594` — chain walker; the
  trailing `%w` at line 570 was already in place to surface
  whatever the leaf returned, but had nothing to surface until this
  fix.
- `pkg/auth/identities/aws/assume_role.go` — patched leaf sites for
  `AssumeRole` and `AssumeRoleWithWebIdentity`.
- `pkg/auth/identities/aws/assume_root.go` — patched leaf site for
  `AssumeRoot`.
- `docs/fixes/2026-04-17-ambient-identity-nil-credentials.md` —
  prior auth-manager UX fix in the same area; same template.
