# Fix: Allow Go toolchain downloads under harden-runner block mode

**Date:** 2026-10-09

## Summary

Jobs that run `actions/setup-go` under StepSecurity harden-runner block mode now
allow every host setup-go uses to download the Go toolchain. This stops
`connect ECONNREFUSED` failures on runners whose image does not already cache
the version pinned in `go.mod`.

## Context

`go.mod` moved to Go 1.26.9 in #3347. Runner images that already cache that
version skip the download, so the same commit passed on some runners and failed
on others. On PR #3345, the `[native ci] workflow groups` jobs, `govulncheck`,
and `website-deploy-preview` all failed in setup-go:

```text
Attempting to download 1.26.9...
connect ECONNREFUSED 54.185.253.63:443
Falling back to download directly from Go
Acquiring go1.26.9 from https://go.dev/dl/go1.26.9.linux-amd64.tar.gz
connect ECONNREFUSED 54.185.253.63:443
```

setup-go reads its version manifest from `raw.githubusercontent.com` and
downloads release assets through GitHub. If that fails, it falls back to
`go.dev/dl`, which redirects to `dl.google.com`. Both hosts resolved to the
same refused address, which appears to be harden-runner's block for unlisted
domains. `release.yml` already documented and allowed the full host set
(`raw.githubusercontent.com`, `release-assets.githubusercontent.com`,
`golang.org`, `go.dev`, and `dl.google.com`), but most other jobs allowed
only part of it.

## Changes

- Added the missing Go download hosts to the `allowed-endpoints` of all 23 jobs
  that reach `actions/setup-go` under block mode, directly or through local
  composite actions such as `setup-atmos-build` and `setup-go-cache`. Each job
  gained only the hosts it lacked. harden-runner and block mode stay in place.

## Validation

- Scanned every workflow job that runs harden-runner in block mode and reaches
  `actions/setup-go` through any local composite action. Before the change,
  23 jobs lacked at least one required host; afterward, none did.
- Parsed every workflow file as YAML and ran `actionlint` on the edited
  workflows with no findings.
- The [preview build on the corrected PR commit](https://github.com/cloudposse/atmos/actions/runs/37987721019)
  passed. The subsequent [preview deployment](https://github.com/cloudposse/atmos/actions/runs/37989333932)
  still failed during Go setup, before AWS credentials or artifact deployment.
  Its log confirms the allowlist lacked the new hosts: this `workflow_run`
  deployment uses the workflow from `main`, not the PR branch.

## Follow-ups

Land the workflow allowlist fix on `main` before expecting preview deployments
to consistently pass on runners without the pinned Go version cached. Then
trigger a fresh preview build so its deployment uses the corrected workflow.
Retrying the old deployment alone does not apply the PR's workflow changes.
