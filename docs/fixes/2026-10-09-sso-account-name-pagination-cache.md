# Fix: Paginate and cache SSO account-name resolution

**Date:** 2026-10-09

## Summary

Permission-set identities configured with only `principal.account.name` now search
all SSO account pages and cache successful name-to-ID resolutions across commands.
This fixes [#3343](https://github.com/cloudposse/atmos/issues/3343).

## Context

`resolveAccountID` called `ListAccounts` once and ignored `NextToken`. Accounts
beyond the first page incorrectly produced an "account not found" error. The
reporter's observed page size was ten; the defect does not depend on that size.
Explicit account IDs bypass this lookup, and automatic identity provisioning
already paginates account discovery and supplies IDs.

The lookup is not restricted to interactive login. Commands such as Terraform
operations call the auth manager, which deliberately re-authenticates the final
identity even when persistent credentials exist. Its process-local credential
cache only avoids repeated authentication within one invocation. Consequently,
a name-only permission-set target can enumerate SSO accounts on each command.
Some auth commands first reuse valid cached credentials and avoid this path.

## Changes

- Use the AWS SDK `ListAccounts` paginator, returning immediately on an exact
  name match and reporting "not found" only after exhausting the pages.
- Preserve explicit-ID precedence and the original AWS error chain on any page.
- Persist successful resolutions under the Atmos XDG cache's
  `aws-sso/account-ids/` directory. Entries expire after one hour and are scoped
  by account name, auth realm, SSO region, endpoint, and the access-token digest.
  Changing the session causes a cache miss; replacing its entry avoids creating
  a new file for every login. No bearer tokens are stored in this cache.
- Use atomic writes, private directories (`0700`), and private files (`0600`).
  The existing `aws-sso` exclusion keeps these entries out of CI cache archives.
  Missing, expired, malformed, or unavailable cache storage falls back to SSO.
  Missing accounts and API failures are never cached.
- Preserve credential acquisition and validation: caching skips account listing,
  while `GetRoleCredentials` still authenticates against AWS. Account-name
  changes within a session can take up to one hour to be reflected.

## Validation

- Before the fix, new regression tests reproduced missed later-page accounts,
  premature "not found" responses, and lost later-page API errors.
- After the fix, focused pagination and cache tests pass, covering persistence
  across identity instances, isolation, expiry, malformed entries, private file
  permissions, storage failures, and uncached lookup failures.
- `go build ./...` and `TEST=./pkg/auth/... atmos test` passed after adding
  persistent caching; all auth packages passed.
- `atmos lint --changed` passed for the pagination change. The final
  `custom-gcl` run against a patch including all modified and new Go files
  reported zero issues. An unfiltered AWS identity package lint run reported
  24 existing findings outside the edited lines (including user/webflow code
  and deprecated credential-store calls in existing tests).
- The repository-wide `atmos test` run reported a failure outside auth:
  `pkg/ci/startup: TestPrintStartupStatus_PrintsWhenInCI` expected "Atmos version"
  but captured an empty string. No CI-startup code was changed. The remaining
  broad run was stopped after this failure; it did not complete successfully.
- The fix-log validator and `git diff --check` passed.
- No live AWS SSO session was used; tests exercise the real SDK against local
  HTTP fixtures and isolated temporary cache directories.

## Follow-ups

None.
