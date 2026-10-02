# Fix: CI failures from the credential_process hardening (test I/O hang, hint wording, Floci STS)

**Date:** 2026-10-02

## Summary

The first CI run of the credential_process hardening (#3249) failed in three places. None of them was a defect in
the shipped behavior. A test helper held `go test`'s output open on Linux and Windows, a new hint reused the exact
phrase that CLI tests use to detect the interactive selector, and the Floci version CI runs answers
`sts:GetCallerIdentity` with HTTP 500 for credentials issued by its own `GetSessionToken`.

## Context

- `pkg/auth/cloud/aws` failed with `*** Test I/O incomplete 4m0s after exiting` on the Linux and Windows
  acceptance shards and the race shard. `TestRetrieveProcessCredentials_TimeoutIsPromptWhenGrandchildHoldsStdout`
  runs the test binary as a fake helper that spawns a grandchild and then sleeps for an hour. On macOS `sh -c`
  execs the single command, so the timeout kills the helper itself. Dash (Linux) and `cmd.exe` (Windows) fork
  instead, so the timeout kills only the shell, and the orphaned helper kept the test binary's inherited stderr
  open, which made `go test` wait four minutes for I/O after the test passed.
- `TestInteractiveIdentitySelection` and `TestCIEnvironmentDetection` (Linux, macOS, Windows) assert that the
  output of a non-interactive run does not contain "Select an identity", the selector's title. The new
  prompt-unavailable hint in `pkg/auth/manager.go` started with "Select an identity explicitly with
  --identity=<name>", so the assertion matched the hint, not a selector.
- `[floci] go e2e` failed `TestAWSCredentialProcessFlociE2E_Producer` and `_Consumer` with
  `GetCallerIdentity ... StatusCode: 500 ... InternalFailure: Unexpected error: null`. This already happened on
  the branch's original commit. The CI service container is pinned to digest `d2ecc80`, which is Floci 1.5.33
  although its comment said 1.5.23, while local runs used the real 1.5.23 digest `c88ec20` through testcontainers
  and passed. Reproduced directly with the AWS CLI against 1.5.33: `GetCallerIdentity` works with the raw keys but
  fails for credentials from Floci's own `GetSessionToken`, and S3 accepts those same credentials.

## Changes

- `pkg/auth/cloud/aws/files_test.go`: the fake helper that spawns the grandchild closes its inherited stderr and
  sleeps only for the grandchild's lifetime instead of an hour, so an orphaned helper can no longer hold the test
  runner's output.
- `pkg/auth/manager.go`: the prompt-unavailable hint now reads "Pass `--identity=<name>`, or mark an identity as
  default with `default: true` in atmos.yaml".
- `tests/credential_process_floci_test.go`: the credential check tolerates only Floci's specific
  `InternalFailure` / "Unexpected error: null" response to `GetCallerIdentity` and logs it; any other error still
  fails. The S3 create, list, and delete round trip remains a hard check that the helper's credentials work.
- `tests/floci_containers_test.go` and `.github/workflows/test.yml`: local testcontainers now use the same Floci
  digest as the CI service container, and the CI comment names the real version (1.5.33), so local runs reproduce
  CI.

## Validation

- Forced a forking shell locally (`<helper>; :`) to reproduce the Linux behavior: with the old helper `go test`
  was still blocked after 45 seconds; with the fix the package finished in 7 seconds.
- `go test ./pkg/auth/... ./cmd/auth/user/` passed.
- `go test ./tests -run 'TestInteractiveIdentitySelection|TestCIEnvironmentDetection'` passed.
- Against Floci 1.5.33 (the CI digest): before the change `TestAWSCredentialProcessFlociE2E_Producer` and
  `_Consumer` failed with the same 500; after it, all four `TestAWSCredentialProcessFlociE2E_*` tests pass, with
  the GetCallerIdentity bug logged.

## Follow-ups

None.
