# Fix: CI apt installs fail over to a reachable mirror under harden-runner

**Date:** 2026-10-08

## Summary

The `[floci] go e2e` job failed before running any test because `apt-get install libudev-dev`
got a `502 Proxy Error` from the Azure Ubuntu mirror and had no reachable fallback. The
workaround meant to steer apt off the Azure mirror never took effect on GitHub-hosted runners.
All three `libudev-dev` install steps now use a shared local action,
`.github/actions/apt-install`. It rewrites the mirror list apt actually reads, so apt fails over
between two plain-HTTP mirrors that the jobs' egress allowlists permit, and it retries the
install with a cooldown.

## Context

GitHub's Ubuntu runner images (see `configure-apt-sources.sh` in `actions/runner-images`) replace
the Azure URI in `/etc/apt/sources.list.d/ubuntu.sources` with
`mirror+file:/etc/apt/apt-mirrors.txt`. That mirror list contains:

```text
http://azure.archive.ubuntu.com/ubuntu/   priority:1
https://archive.ubuntu.com/ubuntu/        priority:2
https://security.ubuntu.com/ubuntu/       priority:3
```

Two consequences:

1. Our steps ran `sed -i 's|http://azure.archive.ubuntu.com/ubuntu|http://archive.ubuntu.com/ubuntu|g'`
    on `ubuntu.sources`. That file no longer contains the Azure URI, so the substitution matched
    nothing and apt kept using the Azure mirror first. The run log confirms this: after the
    `sed`, apt still fetched from `azure.archive.ubuntu.com`.
2. The `floci-go` job (and the cache warmup job) run harden-runner with `egress-policy: block`
    and allowlist only `archive.ubuntu.com:80` and `azure.archive.ubuntu.com:80`. The runner's
    fallback mirrors are HTTPS (port 443), so they are blocked. When the Azure mirror returned a
    502, apt logged `Ign` for both fallbacks and the install failed with exit code 100.

The three install steps (`race` and `floci-go` in `test.yml`, and the linux leg of
`setup-go-cache-warmup.yml`) had also drifted apart: only some disabled SRV lookups or removed
the Chrome apt source.

## Changes

- **New local action `.github/actions/apt-install`**, which takes a `packages` input. It:
  - rewrites `/etc/apt/apt-mirrors.txt` (when present) to `http://archive.ubuntu.com/ubuntu/`
    first and `http://azure.archive.ubuntu.com/ubuntu/` second. Both are plain HTTP, both are
    already allowlisted, and both carry every pocket, including `-security`;
  - disables apt's SRV lookups in `apt.conf.d` (previously only in the warmup job);
  - removes the runner's unrelated `google-chrome.list` source (previously not in `floci-go`);
  - runs `apt-get update` and `install` with the existing per-request retries and timeouts, and
    retries the pair up to three times with a 30-second cooldown, matching
    `go-mod-download-retry`.
- **Call sites:** `test.yml` (`race`, `floci-go`) and `setup-go-cache-warmup.yml` now call the
  action instead of carrying their own inline copies.

## Validation

- Read the runner image's `configure-apt-sources.sh` through the GitHub API to confirm the
  mirror-list mechanism and its contents.
- Checked the failing job log (run 37701940151, job 113071045025): after the `sed`, apt still
  hit `azure.archive.ubuntu.com`, and the HTTPS fallbacks were ignored before the 502.
- Pre-commit hooks on the changed files: see the PR.
- The action only runs on GitHub-hosted Ubuntu runners, so the real check is the PR's own CI:
  the `race` and `[floci] go e2e` jobs in the Tests workflow.

## Follow-ups

None.
