# Fix: `atmos aws credential-process` honors `--min-validity` for standalone identities and never prompts

**Date:** 2026-10-02

## Summary

A field test of the produce side of `credential_process` support (`atmos aws credential-process` and
`atmos auth env --format=credential-process`) found that `--min-validity` was ignored for `aws/user` and
`aws/credential-process` identities, that the AWS CLI could hang forever on an invisible identity selector, that
identity-resolution errors were confusing or lost their hints, and that credentials could leak through a briefly
world-readable file and through `--cast` recordings. All of these are fixed.

## Context

The AWS CLI runs `credential_process` once per invocation and captures the helper's stderr. Anything the helper
needs to show a human is therefore invisible, and anything that waits for input hangs the `aws` command.

- `credentialprocess.Produce` applied `--min-validity` only to its own cached-credential check. It then called
  `Authenticate`, and `aws/user` (any unexpired session) and `aws/credential-process` (a hard-coded 15 minute
  buffer) returned their own cache. A helper returning 20 minute credentials, queried with
  `--min-validity=30m`, was never re-run.
- The 15 minute value existed in four places: the flag default string, `DefaultMinValidity`,
  `credentialProcessReuseBuffer`, and the manager's `minCredentialValidityBuffer`.
- `atmos auth env --format=credential-process` ignored `ATMOS_AWS_CREDENTIAL_PROCESS_MIN_VALIDITY`, had no
  `--min-validity`, silently ignored `--login=false`, and could open the interactive selector.
- `--identity=false` leaked the internal sentinel (`failed to authenticate identity '**DISABLED**'`). With no
  identity and no default, the message repeated itself and had no hint. With stdin on a terminal,
  `GetDefaultIdentity` opened the selector, which was invisible to the AWS CLI, so `aws --profile` hung
  (reproduced). When the identity lived in a profile that was not loaded, the manager's explanation and hint were
  dropped by a double-`%w` wrap.
- `WriteFile` used `os.WriteFile` and then `os.Chmod`, so an existing 0644 file briefly held the secrets.
- `data.WriteUnmasked` passed the raw content to `io.RecordMaskedOutput`, so `--cast`/`ATMOS_CAST` recordings
  captured the credentials.
- `NewProcessCredentials` returned `ErrIdentityNotAWS` for AWS credentials with an empty key.

## Changes

- **Minimum validity reaches standalone identities.** `pkg/auth/types/context.go` adds
  `WithMinCredentialValidity`, `MinCredentialValidity`, and `MinCredentialValidityOr`, following the existing
  `WithForceAWSWebflow` and `WithAllowPrompts` context precedent. `Produce` sets it. `aws/credential-process`
  (`reusableFileCredentials`) and `aws/user` (new `sessionStillValid`) refresh a cache that does not satisfy it.
  The cloud-agnostic `types.ExpiresWithin(ICredentials, time.Duration)` (built on `GetExpiration`) is the
  shared check. If the source still issues shorter credentials, `Produce` returns them and logs at debug level.
  Chains already re-authenticate the target, and a standalone chain root now also sees the requested validity.
- **One 15 minute constant.** `types.DefaultMinCredentialValidity` is used by the manager's
  `minCredentialValidityBuffer`, `credentialprocess.DefaultMinValidity`, the `aws/credential-process` cache
  default, and both flag defaults (`credentialprocess.FormatMinValidity` renders `15m`). The flag name, env var,
  and `ParseMinValidity` moved to `pkg/auth/cloud/aws/credentialprocess` and are shared by both commands.
- **Alias parity.** `atmos auth env --format=credential-process` gains `--min-validity` (through `pkg/flags`) and
  honors `ATMOS_AWS_CREDENTIAL_PROCESS_MIN_VALIDITY`. `--login=false` with this format is an explicit
  `ErrInvalidFlagValue` error, because the format must authenticate when credentials are missing or about to
  expire; silently authenticating after the user said not to would be misleading. `--min-validity` with any other
  format is also an error rather than a silent no-op. The alias resolves the identity with the same
  non-interactive rules as the canonical command.
- **Identity resolution never prompts.** `credentialprocess.ResolveIdentity` rejects `--identity=false` (and
  false-like values) and bare `--identity` with the new `ErrCredentialProcessIdentityRequired`, and resolves the
  default identity from `GetIdentities` instead of `GetDefaultIdentity`, so a terminal never gets a selector.
  No default and multiple defaults return the existing sentinels as one sentence plus a hint that names
  `--identity=<name>` and `atmos auth list`. The `--identity` flag help on `credential-process` and the docs no
  longer advertise interactive selection.
- **Hints survive.** `Produce` wraps authentication failures with a single-cause `errUtils.Build(...).WithCause`,
  so the manager's explanation and profile hints reach the formatted output without relying on multi-`%w`
  traversal.
- **Atomic `--output-file`.** `WriteFile` creates a 0600 temp file in the target directory and renames it over the
  target, removing the temp file on failure.
- **Cast recording.** `data.WriteUnmasked` records `Masker().Mask(content)` while stdout stays unmasked.
- **Sentinels.** `NewProcessCredentials` returns `ErrIdentityCredentialsNone` for nil credentials and the new
  `ErrAWSCredentialsIncomplete` for empty keys. `ErrIdentityNotAWS` is kept for non-AWS credentials.
- **Docs and tests.** Updated `aws-credential-process.mdx`, `auth-env.mdx`, `pkg/auth/docs/ARCHITECTURE.md`, and
  the `atmos auth env --help` golden snapshot (regenerated). Tests added or tightened in
  `pkg/auth/cloud/aws/credentialprocess`, `pkg/auth/types`, `pkg/auth/identities/aws`, `pkg/auth/cloud/aws`, `pkg/data`,
  `cmd/aws`, and `cmd/auth`, including negative-path cases.
- **AWS code stays in the AWS tree.** The producer package lives at `pkg/auth/cloud/aws/credentialprocess` (moved
  with `git mv` from `pkg/auth/credentialprocess`), and the expiry check is a generic helper in
  `pkg/auth/types/expiry.go` rather than a method on `AWSCredentials`, so no AWS-specific logic was added to the
  cloud-agnostic `pkg/auth` layers.
- **Profile hint rendering.** `pkg/auth/profile_fallback.go` wrapped the already-quoted profile list in a second
  pair of backticks, so the "defined in these profiles" hint rendered as mangled code spans. The list is quoted
  once, and both re-run hints use the `--profile=<name>` form.

## Validation

- `go build ./...` passed.
- `go test ./pkg/auth/... ./cmd/aws/... ./cmd/auth/... ./pkg/io/... ./pkg/data/... ./errors/...` passed.
- `go test ./tests -run 'TestCLICommands/atmos_auth_env_--help' -regenerate-snapshots` regenerated the one changed
  snapshot; the diff only adds the `--min-validity` flag and re-aligns the flag column.
- Confirmed the new cast-recorder test fails without the masking change.
- `./custom-gcl run --new-from-rev=origin/main` on the changed packages reported no findings on the lines of this
  change (remaining findings are in code owned by other changes).
- `cd website && npm run build` succeeded.
- Regenerated the `atmos-aws--help`, `atmos-auth-env--help`, and `atmos-aws-credential-process--help` screengrab
  casts with `casts generate screengrabs cli --filter`; `casts validate screengrabs cli` passed.
- `TestBuildProfileSuggestionError_MultipleCandidates` now asserts the profile list is quoted exactly once
  (it contains a double backtick with the old format string) and the single-candidate tests assert
  `--profile=<name>`; `go test ./pkg/auth/` passed.
- Live field-test fixture (fake helper, file keyring): with `cp-20m`, `--min-validity=30m` did not re-run the helper
  before the change (counter unchanged) and re-ran it once after; the default and `--min-validity=5m` still reuse
  the cache; the alias with `--min-validity=30m` or the env var re-runs the helper; `--identity=false`, bare
  `--identity`, and no default identity fail immediately with a hint; with stdin on a pty the old binary hung on the
  selector and the new binary exited with code 1 within the 10 second guard; `--output-file` over an existing 0644
  file ends 0600 with no leftover temp file.
- Final combined run on the branch with all of the 2026-10-02 auth fixes applied: `go build ./...`; `go test` for
  `./errors/... ./pkg/auth/... ./cmd/auth/... ./cmd/aws/... ./cmd/azure/... ./cmd/gcp/... ./pkg/devcontainer/...
  ./pkg/store/... ./pkg/process/... ./pkg/io/... ./pkg/data/...`; `go test -short ./cmd/... ./internal/exec/...`;
  `go test ./tests -run 'TestCLICommands/atmos_(auth|aws)'`; `GOOS=windows go vet` on the auth packages; and
  `./custom-gcl run --new-from-rev=origin/main` (0 issues). All passed.

## Follow-ups

None.
