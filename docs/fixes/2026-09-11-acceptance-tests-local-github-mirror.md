# Fix: acceptance tests fetch cloudposse/atmos git sources from a local, token-validating git-over-HTTP mirror

**Date:** 2026-09-11

## Summary

The acceptance suite's vendor and demo cases cloned `github.com/cloudposse/atmos.git//examples/...` over the
network on every shard. They now clone from a local mirror of this checkout's `examples/`, served over git's real
smart-HTTP protocol by `git http-backend`, with the redirect done by git (`url.<mirror>.insteadOf` rules in a
gitconfig delivered through `GIT_CONFIG_GLOBAL`) and the credentials validated by the mirror. Atmos itself is
unaware: it sees the same URLs, runs the same detector, and injects the same token as against real GitHub.

## Context

- Hosted-runner DNS and connect blips mid-clone failed a shard several times a week
  (`2026-09-02-vendor-pull-dns-resolution-flake.md`, `2026-09-07-jit-source-network-flakes.md`).
- GitHub introduced protections against unauthenticated traffic to public repositories
  (https://github.com/orgs/community/discussions/206581#discussioncomment-18269356), so anonymous fetches now
  fail or rate-limit in ways that look like 401s and 404s. The same pressure shows up downstream in
  hashicorp/terraform#39130 (https://github.com/hashicorp/terraform/issues/39130), where users ask for SSH module
  fetching because unauthenticated HTTPS to GitHub is rate limited. Unauthenticated GitHub access is a canary
  concern, not something every PR shard should depend on.
- The first CI run of the mirror also fixed the toolchain bootstrap's own transient failure mode
  (`peteretelej/tree` release asset returning a one-off 404) by giving the install step a bounded retry
  (a later PR in the same stack).

## Design

- `tests/testhelpers/gitmirror`: `Build` publishes a bare repo of `examples/` (via `git init --bare` + `push`;
  a local `git clone --bare` failed on the macOS runners with "trying to write ref ... with nonexistent object");
  `Serve` runs `git-http-backend` behind `httptest` through `net/http/cgi`, requires HTTP Basic auth with a
  registered token unless `AllowAnonymous()`, rejects pushes, and records requests; `WriteGitConfig` emits one
  `insteadOf` rule per registered token in the exact userinfo form atmos hands git
  (`https://x-access-token:<token>@github.com/cloudposse/atmos.git`) plus the anonymous https and ssh forms.
- Rules are repo-scoped. An owner-wide rule also captured terraform's own module fetch of
  `terraform-null-label` (pinned to an upstream commit no mirror can reproduce) and failed the `ci_summary`
  plan cases.
- Rules go through `GIT_CONFIG_GLOBAL`, never `GIT_CONFIG_*` env: `pkg/downloader/custom_git_detector.go`
  scans the env form for broker rewrites and would skip URL token injection, which changed the
  credentials-leakage golden and removed the injected-token path from the test. With the file form the golden
  is byte-identical to `main`.
- The harness sets a static `GITHUB_TOKEN` when none is ambient, so atmos never falls back to `gh auth token`
  (an unknown value would miss the rules and silently go live). Ambient CI tokens are registered with the
  mirror too, so the mixed git plus OCI case keeps working.
- The shared server allows anonymous fetches because `cloudposse/atmos` is public and an explicit
  `git::https://` source bypasses atmos's detector (no injection), exactly as in production; the
  401-challenge path that proves token validation end to end is covered by `gitmirror`'s own tests.

## Verification

- `go test ./tests -run 'TestCLICommands/(atmos_vendor_pull|atmos_vendor_pull_git|atmos_vendor_pull_with_globs|atmos_vendor_pull_with_custom_detector_and_handling_credentials_leakage)$'`
  passes with no ambient token, with a dummy ambient token, and with `HTTPS_PROXY` pointed at a closed port.
- `tests/snapshots/` is byte-identical to `main`.
- Related PRs: #3105 (this change), #3107 (GitHub Enterprise Server support, the env seam the HTTP façade
  uses), #3109 (live-GitHub canaries, `ATMOS_TEST_OFFLINE`, toolchain install retry), #3122 (GitHub HTTP
  façade for toolchain, registry, and raw fetches).
