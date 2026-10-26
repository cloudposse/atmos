# github/sts Token Silently Shadowed by CustomGitDetector

**Date:** 2026-06-01
**Introduced by:** PR #2546 (Atmos Pro STS — just-in-time GitHub tokens for CI)
**Severity:** High — private repos covered by a `github/sts` mint fail to clone in CI, despite a valid least-privilege token being minted
**Reproducer:** `pkg/downloader/broker_insteadof_test.go` (`TestCustomGitDetector_BrokerInsteadOf_NotShadowed`)

---

## Why this is a fix doc (and not a blog post / changelog entry)

This PR makes a just-shipped feature (`github/sts`, #2546) work as it was already documented to
work. It introduces **no net-new user-facing capability** — there is no new command, flag, or
feature to announce. The `token_env` default and the `{{ .owner }}` template syntax are corrections
in service of fixing the broken composition within the same unreleased feature line, not additions on
top of a working one. Per the repo's label decision tree that makes it a `patch`, which does **not**
require a `website/blog/` post or a roadmap milestone. The rationale and design are captured here in
`docs/fixes/` instead, alongside the other regression/fix write-ups.

---

## Symptom

A CI job resolving a remote stack `import:` from a second private repo failed:

```text
failed to download remote import 'github.com/<org>/<repo>//stacks/catalog/queue'
... error downloading 'https://x-access-token:redacted@github.com/<org>/<repo>?depth=1'
... /usr/bin/git exited with 128: remote: Repository not found.
```

The repo's auth config defined an `atmos/pro` provider + `github/sts` integration scoped to the
second repo, with `git_config_mode` and `token_env` unset. A least-privilege token **was** minted
correctly — but the wrong (ambient `GITHUB_TOKEN`) token was being used, so GitHub returned 404.

---

## Root Cause

`github/sts` (env mode, the default) does not put a token in the URL. It mints a token and exports
per-owner git `insteadOf` rewrites via `GIT_CONFIG_*` env vars (`pkg/auth/integrations/github/sts.go`
`Environment()`), which the ambient broker `os.Setenv`s before the first remote read
(`pkg/auth/broker/broker.go:96`). Git then rewrites `https://github.com/<owner>/…` →
`https://x-access-token:<minted>@github.com/<owner>/…`.

Two overlapping footguns in the Atmos-native go-getter path
(`pkg/downloader/custom_git_detector.go`) defeated this:

### Footgun 1 — shadowing

`settings.inject_github_token` defaults to `true`. `injectToken` writes
`x-access-token:<TOKEN>@` directly into the URL. Once the URL carries userinfo, git's `insteadOf`
**left-side** (`https://github.com/<owner>/`) no longer prefix-matches, so the rewrite is silently
bypassed. The injected token is the ambient `GITHUB_TOKEN`/`ATMOS_GITHUB_TOKEN` (the Actions token,
scoped only to the current repo) — which has no access to the separate repo → 404. The minted
least-privilege token is never used.

### Footgun 2 — `token_env` / `ATMOS_PRO_GITHUB_TOKEN` never reached the in-process git path

Setting `spec.token_env: ATMOS_PRO_GITHUB_TOKEN` exported the minted token via the broker's
`os.Setenv`, but `CustomGitDetector.resolveToken` read the **struct field**
`d.atmosConfig.Settings.AtmosProGithubToken`, populated at startup **before** the broker runs —
whereas `pkg/http/client.go` reads the **live** `os.Getenv("ATMOS_PRO_GITHUB_TOKEN")`. Nothing wrote
the minted token back into `atmosConfig.Settings`, so the bridge fixed HTTP/`gh`/terraform-subprocess
paths but not the in-process git clone. Inconsistent and broken for git.

The only working fix was the config workaround `settings.inject_github_token: false` — and needing it
is the footgun: two features that should compose instead fought, and the wrong token won.

---

## Fix

All three changes make `github/sts` compose with the default `settings.inject_github_token: true`.

### 1. Don't shadow a broker `insteadOf` (core fix)

`pkg/downloader/custom_git_detector.go` — new `brokerInsteadOfMatchesURL(parsedURL, host)` scans the
live `GIT_CONFIG_*` env (`GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_n` / `GIT_CONFIG_VALUE_n`) for an
**https** `insteadOf` rewrite whose host + first path segment (owner) match the URL. "Match" is git's
own term for `insteadOf` (any URL beginning with the rewrite's value is rewritten). `injectToken`
skips URL injection when one matches, so git's rewrite — carrying the correct minted token — wins.
The check is host/owner-scoped: a repo under a **non-minted** owner still gets the ambient token
injected exactly as before (an ssh-only rewrite never suppresses an https injection).

### 2. Make the `ATMOS_PRO_GITHUB_TOKEN` bridge consistent

`resolveToken` falls back to live `os.Getenv("ATMOS_PRO_GITHUB_TOKEN")` when
`Settings.AtmosProGithubToken` is empty, mirroring `pkg/http/client.go` — so the broker-set token
reaches the in-process git path too.

### 3. Default `token_env` to `ATMOS_PRO_GITHUB_TOKEN`

`pkg/auth/integrations/github/sts.go` `parseSTSSpec` now defaults `token_env` to
`ATMOS_PRO_GITHUB_TOKEN` (was empty), in both env and file mode. A single-owner mint therefore
bridges automatically to `gh`/REST and (via fix #2) the in-process git path, with no manual config.
Multi-owner mints under the literal default skip the bare var (logged at debug, not warn) and rely on
the `GIT_CONFIG_*` rewrites, which fix #1 preserves.

### 4. Standard Go-template syntax for `token_env`

The ad-hoc `{owner}` placeholder is replaced with Atmos's standard `{{ .owner }}` Go-template syntax
(rendered via stdlib `text/template`, then sanitized into a valid env var name); `.host` is also
available. Done now, before #2546 is widely adopted, to avoid an inconsistent mini-syntax.

---

## Tests

- `pkg/downloader/broker_insteadof_test.go` — simulated-broker e2e (written first, confirmed to
  reproduce the failure before the fix): a matched owner is **not** shadowed; a different owner is
  **still** injected.
- `pkg/downloader/broker_insteadof_helper_test.go` — table-driven `brokerInsteadOfMatchesURL`
  (https match, ssh-only no-match, different owner/host no-match, no `GIT_CONFIG_COUNT`) and the
  `resolveToken` live-env fallback (live used when struct empty; struct wins when set).
- `pkg/auth/integrations/github/sts_test.go` — `token_env` default for single owner (env + file
  mode), multi-owner skip, `.host` template, invalid-template graceful skip.
- Existing precedence tests guarded with `t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")` to stay
  deterministic.

---

## Verification

```bash
go build ./... && go test ./pkg/downloader/... ./pkg/auth/...
cd website && npm run build
```

End-to-end: with the fix and `inject_github_token` left at its default `true`, a remote `import:` of
a repo covered by the mint clones via the STS `insteadOf` token (injection skipped) and succeeds; a
repo under a non-minted owner still gets the ambient token injected (unchanged behavior).

---

## Related

- PR #2546: Atmos Pro STS (introduced `github/sts`; this fix targets it)
- `docs/prd/atmos-pro-sts.md`: PRD (updated — `token_env` default, shadow-avoidance note)
- `pkg/downloader/custom_git_detector.go`: `brokerInsteadOfMatchesURL`, `injectToken`, `resolveToken`
- `pkg/auth/integrations/github/sts.go`: `parseSTSSpec` default, `renderTokenEnvName`
- `pkg/http/client.go`: `GetGitHubTokenFromEnv` (the live-env precedent mirrored here)
