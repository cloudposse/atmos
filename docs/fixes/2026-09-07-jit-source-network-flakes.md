# Fix: JIT source provisioning retries DNS/connect failures; JIT tests clone a local repo instead of GitHub

**Date:** 2026-09-07

## Summary

`Acceptance Tests (windows, shard 2/10)` on #3069 failed in `TestJITSource_PackerOutput` with:

```text
failed to download file: error downloading '***github.com/aws-samples/amazon-eks-custom-amis?depth=1&ref=main':
git command exited with non-zero status: ... fatal: unable to access
'https://github.com/aws-samples/amazon-eks-custom-amis/': Failed to connect to github.com:443 after 61 ms:
Could not connect to server
```

The same job's checkout step had already hit `Could not resolve host: github.com` seconds earlier and
recovered on `actions/checkout`'s own retry. Nothing in the PR touched source provisioning. This is the same
family as `2026-09-02-vendor-pull-dns-resolution-flake.md` and `2026-08-10-github-transient-error-tls-cert-flake.md`,
and it recurs several times a week across Windows shards.

## Context

On Windows, harden-runner's agent enforces egress with a DNS proxy on `127.0.0.1:53` and a WFP allow-rule per
resolved IP (`2026-09-03-harden-runner-windows-dns-restore-race.md`). Under the process churn of a test shard
(the agent logged hundreds of `pid reused` warnings in this job) the proxy occasionally drops a query
(`Could not resolve host`) or the allow-rule lands after the client already connected — a 61 ms refusal, not a
network timeout. Both clear on the next attempt.

Two things in this repository turned that blip into a red build:

- The git getter's retry predicate (`isRetryableGitError`, `pkg/downloader/get_git.go`) did not recognise
  `could not resolve host` or `could not connect to server`, so even a source with `retry:` configured would not
  have retried. And JIT source provisioning (`pkg/provisioner/source/vendor.go`) only passed a retry policy when
  the source set `retry:` explicitly — there was no bounded default, unlike the brokered-auth path. A user's
  `terraform plan` with a JIT source hits the same blip on a laptop or in CI.
- `TestJITSource_*` (`tests/cli_jit_source_workdir_test.go`) and `TestJITSource_MetadataComponentSubpath*`
  (`tests/cli_source_provisioner_workdir_test.go`) test the JIT provisioning path, not GitHub; cloning
  `terraform-null-label`, `helmfiles` and `amazon-eks-custom-amis` from github.com was incidental. The precondition
  helper `RequireGitHubAccess` only checks connectivity once at the start, so a mid-clone blip still failed the test.

A Gitea/testcontainers stand-in was considered and rejected for these tests: the flake is on Windows, and
GitHub's Windows and macOS hosted runners cannot run Linux containers, so a container-backed fixture would skip
exactly where coverage is needed. A local `git::file://` repository exercises the same go-getter git path
(clone, `ref=`, `depth=1`, `//subpath`) on every OS with no network — the pattern
`tests/cli_remote_imports_test.go` already uses.

## Changes

- `pkg/downloader/get_git.go`: `isRetryableGitError` also matches `could not resolve host`, `no such host`,
  `could not connect to server`, `failed to connect to`, `network is unreachable` and `name resolution`.
  `pkg/downloader/get_git_retry_test.go` covers each, plus a guard that `unable to access ... returned error: 403`
  is still not retried.
- `pkg/provisioner/source/vendor.go`: `downloadGoGetterSource` always passes a retry policy —
  the source's own `retry:` when set, otherwise a bounded default (3 attempts, exponential backoff 1s→8s, 20% jitter).
  Only the git getter's transient-transport predicate triggers a retry; auth failures and missing refs still fail
  fast, and `max_attempts: 1` opts out. `retry_default_test.go` covers the default and the explicit-wins case.
- `tests/jit_source_local_repo_test.go` (new): builds a throwaway git repository with the files the fixtures expect
  (`exports/context.tf`, `releases/nginx-ingress/helmfile.yaml`, `*.pkr.hcl`), tags `0.25.0` and `0.126.0`, branch
  `main`, and rewrites the three upstream repository URIs in a test's *sandboxed* copy of the
  `source-provisioner-workdir` fixture to `git::file://` — subpaths and `version:` untouched. The checked-in
  fixture keeps its real-world URIs.
- `tests/cli_jit_source_workdir_test.go`: `setupJITSourceWorkdirFixture` requires `git` and wires the local repo in.
- `tests/cli_source_provisioner_workdir_test.go`: the two `MetadataComponentSubpath` tests use the same sandbox
  setup instead of `RequireGitHubAccess` + running in the checked-in fixture directory.
- Docs: `terraform`/`helmfile`/`packer` `source` pages state the new default retry behaviour under `retry`.

Not changed: `atmos vendor pull` (`pkg/vendor`) still has no default retry; the `vendor` acceptance fixture is
deliberately real-network (see `2026-09-02-vendor-pull-dns-resolution-flake.md`).

## Validation

- `go test ./tests/ -count=1 -run 'TestJITSource|TestSourceWorkdir'`: all 9 JIT/workdir tests pass in 46 s with
  no network access (macOS, local).
- `go test ./pkg/downloader/ ./pkg/provisioner/... -count=1`: pass, including the new predicate cases and
  `TestEffectiveRetryConfig_*`.
- `custom-gcl run --new-from-rev=origin/main` (pinned to the go.mod toolchain): 0 issues.
- `pnpm run build` in `website/`: succeeds; the only broken-anchor warnings are the three pre-existing ones
  also reported on `main` (`mcp-for-ai-coding-assistants`, `terraform.output`, `terraform.state`).
- Not validated: the retry default against a real dropped-DNS event (not reproducible on demand); the predicate
  strings are taken verbatim from the failing job's git output.

## Follow-ups

None.
