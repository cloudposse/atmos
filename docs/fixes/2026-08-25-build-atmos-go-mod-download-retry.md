# Fix: retry `go mod download` in scripts/build-atmos.sh on transient proxy.golang.org failures

**Date:** 2026-08-25

## Summary

Two CI jobs ("deploy (auto verify + apply)" and "deploy (drift fails)") failed with
`go: <module>@<version>: read "https://proxy.golang.org/...": stream error: stream ID <n>;
INTERNAL_ERROR; received from peer` -- a transient HTTP/2 stream reset from the Go module proxy's
CDN, a different module in each failure, with no code or dependency problem involved.
`scripts/build-atmos.sh`'s `go mod download` call had no retry logic, so a single mid-stream proxy
hiccup failed the whole build outright. Wrapped it in a 3-attempt/15s-backoff retry loop, matching
the convention already used for artifact downloads.

## Context

Both failures happened during `sh scripts/build-atmos.sh default test`, well before any test or
application code runs, purely fetching third-party Go module dependencies. Neither failing
workflow (`planfile-artifacts-e2e.yml`, `planfile-verify-e2e.yml`) nor the module versions
involved (`github.com/extism/go-sdk@v1.7.1`, `github.com/hairyhenderson/gomplate/v3@v3.11.8`) have
anything to do with the PR under active work (`osterman/fix-workdir-h-char-injection`, PR #2985);
`scripts/build-atmos.sh` has zero diff on that branch. Found via attached CI failure logs while
asked to fix failing CI actions.

`go mod download` inherits `GOPROXY='https://proxy.golang.org,direct'`, so Go does have a `direct`
fallback configured -- but that only helps when the proxy is unreachable outright, not when a
request partway through a download gets reset mid-stream (an HTTP/2-level error, not a "proxy is
down" error), which `go mod download` does not appear to retry on its own in this failure mode.

`scripts/build-atmos.sh` is shared across every build path that needs a real `atmos` binary --
native CI jobs (including both failing workflows), `website-preview-build.yml`,
`website-deploy-prod.yml`, `landing-demos.yaml`, and local/dev builds via `atmos build` -- so
hardening it here fixes this failure class everywhere it's used, not just the two jobs that
happened to hit it this time. The repo already has a matching retry convention for exactly this
kind of transient-network-blip problem: `.github/actions/download-artifact-retry`, three attempts
with a 15s cooldown between each, used because `actions/download-artifact` only retries HTTP
5xx/429 and not connection-level failures.

## Changes

- `scripts/build-atmos.sh`: wrapped the bare `go mod download` call in a POSIX-sh `until` retry
  loop (three attempts, 15s sleep between each, matching the artifact-download-retry convention),
  failing loudly with attempt-count context if all three attempts are exhausted. The script's
  `set -eu` doesn't short-circuit this, since a command used as a `while`/`until` condition is
  exempt from `set -e`'s early-exit behavior.
- Follow-up (same day): the initial commit indented the new lines with spaces, matching the rest
  of the file -- but `.editorconfig` declares `indent_style = tab` for `*.sh`, and the whole file
  had actually been space-indented all along. That was never caught before because
  `atmos-validate-editorconfig` only runs `--affected` (files changed since the merge-base), and
  nothing had touched this file in a triggering diff until this fix did. Converted every indented
  line in the file from 2-space indentation to tabs (one tab per indent level, per
  `indent_size = 2`) so the pre-commit hook and CI's "Validation (affected)" job both pass. A
  repo-wide (non-`--affected`) `atmos validate editorconfig` run turned up thousands of unrelated
  pre-existing violations elsewhere (mostly Markdown blog posts) that are out of scope here --
  scoped this fix to the one file `--affected` actually flags, matching what CI runs.

## Validation

- `shellcheck -s sh scripts/build-atmos.sh` -- clean, before and after the tab conversion.
- `sh scripts/build-atmos.sh default test` -- happy path still builds `build/atmos` successfully
  and it runs (`./build/atmos version`), both before and after the tab conversion.
- Isolated the retry loop's control flow with a fake `go` shell function and verified both paths
  directly: (1) failing twice then succeeding on the 3rd attempt recovers and continues normally;
  (2) failing on all 3 attempts prints the failure-count message and exits 1 (does not silently
  swallow a genuine, persistent failure). Re-verified after the tab conversion too.
- `./build/atmos validate --affected --exclude 'tests/fixtures/**' --exclude '**/*.go' --format
  rich` (the exact command the `atmos-validate-editorconfig` pre-commit hook runs) -- passes
  cleanly after the tab conversion; failed with dozens of "Wrong indentation type" errors on this
  file before it.
- Patch-scoped `./custom-gcl run --new-from-rev=origin/main` -- 0 issues.

## Follow-ups

None.
