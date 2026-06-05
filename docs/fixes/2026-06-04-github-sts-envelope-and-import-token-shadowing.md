# github/sts: Pro Envelope Not Unwrapped + Minted Token Shadowed During In-Process Import Resolution

**Date:** 2026-06-04
**Introduced by:** PR #2546 (Atmos Pro STS) and #2557 (the first shadowing fix — see
`2026-06-01-github-sts-token-injection-shadowing.md`, which fixed the *downloader Fetch* path but not
the *eager-detect* path documented here)
**Severity:** High — a cross-repo private `import:` 404s in CI (`remote: Repository not found`) even
though Atmos Pro mints a valid least-privilege token scoped to the target repo
**Reproducers:**
- `pkg/auth/integrations/github/sts_test.go` — `TestGitHubSTSMint_DecodesCanonicalEnvelope`
- `pkg/stack/imports/remote_broker_test.go` — `TestRemoteImporter_GitSubdir_ProvisionsBrokerBeforeDetect`
- `pkg/downloader/broker_insteadof_filemode_test.go` — file-mode `include.path` guard
- `pkg/pro/envelope_test.go` — the canonical-envelope decoder, incl. the flat-payload canary

---

## Why this is a fix doc (and not a blog post / changelog entry)

Like #2557, this makes the just-shipped `github/sts` feature (#2546) work as already documented. No
net-new user-facing capability — no new command, flag, or config surface. Per the repo's label
decision tree that is a `patch`, which does **not** require a `website/blog/` post or a roadmap
milestone. The rationale, the verified design analysis, and a recommended follow-up are captured here.

---

## Symptom

CI (`atmos describe affected`) resolving a remote stack `import:` from a second private repo:

```text
failed to download remote import 'github.com/cloudposse-sandbox/atmos-pro-qa-2-xl//stacks/catalog/queue'
... error downloading 'https://x-access-token:redacted@github.com/cloudposse-sandbox/atmos-pro-qa-2-xl?depth=1'
... /usr/bin/git exited with 128: remote: Repository not found.
```

The consuming repo's config (`cloudposse-sandbox/atmos-pro-qa-2/atmos.yaml`) is minimal:

```yaml
auth:
  providers: { atmos-pro: { kind: atmos/pro, base_url: https://qa-2.atmos-pro.com } }
  identities: { atmos-pro: { kind: atmos/pro, via: { provider: atmos-pro } } }
  integrations:
    github-sts:
      kind: github/sts
      via: { identity: atmos-pro }
      spec:
        repos: [cloudposse-sandbox/atmos-pro-qa-2-xl]   # single repo, single owner
```

No `git_config_mode` (→ default `env`), no `token_env` (→ default `ATMOS_PRO_GITHUB_TOKEN`). The
Atmos Pro server audit confirmed it **minted 1 token** with the target repo in `repositories[]` and
zero exclusions — so the server side is correct. The failure is entirely client-side.

The bug presented in **three** layers, peeled one after another (each fix exposed the next):

1. **Before the envelope fix:** the CLI logged `GitHub STS: no tokens granted (0 excluded)` and fell
   back to the ambient `GITHUB_TOKEN` → 404.
2. **After the envelope fix:** the token minted and was received, but the clone **still** 404'd
   because the ambient `GITHUB_TOKEN` was applied to the cross-repo URL instead of the minted token.
3. **After the timing fix:** auth finally worked — the CI log shows the broker `insteadOf` matched,
   a clean URL (`git::https://github.com/cloudposse-sandbox/atmos-pro-qa-2-xl?depth=1`, no userinfo),
   and **no 404** — but the clone then failed on a *non-auth* error:
   `fatal: empty string is not a valid pathspec. please use . instead if you meant to match all paths`.

---

## Root Cause

### 1. The Pro API envelope was not unwrapped (`mint()`)

`pkg/auth/integrations/github/sts.go` `mint()` decoded the HTTP body directly into the flat
`stsResponse` struct (top-level `json:"tokens"`/`json:"excluded"`). But **every** Atmos Pro route
returns the canonical envelope:

```json
{ "success": true, "status": 200, "data": { "tokens": [...], "excluded": [...] } }
```

Tokens are nested under `data`, so `out.Tokens` always decoded empty → `persistAndReport` hit
`len(resp.Tokens) == 0` → "no tokens granted", never wrote the git config, never persisted a token.
HTTP was 200, so no error surfaced. `mint()` was the only Pro call bypassing the shared
`AtmosApiResponse` envelope that `ExchangeOIDCToken`/`LockStack` already use.

### 2. The eager detect runs *before* the broker (timing) — the real cross-repo failure

`atmos describe affected` resolves remote stack imports **in-process** via
`pkg/stack/imports/remote.go` `RemoteImporter.resolveGitSubdir`, which calls `detectGitSource` →
`CustomGitDetector.Detect` **directly, before** `r.downloader.Fetch`.

The credential broker (`pkg/auth/broker`) is what `os.Setenv`s the github/sts `Environment()` —
**both** the per-owner `GIT_CONFIG_*` `insteadOf` rewrites **and** the raw `ATMOS_PRO_GITHUB_TOKEN`
— into the process. But the broker only runs inside the downloader (`gogetter_downloader.go`
`NewClient` → `broker.EnsureCredentials`). So at the moment of the **eager** `detectGitSource`,
neither was set in the process env. `resolveToken` therefore fell through to the ambient
`GITHUB_TOKEN` (the Actions token, scoped to the calling repo only), baked it into the URL as
`x-access-token:<ambient>@…`, and GitHub returned 404 for the cross-repo target.

This is **distinct from** #2557. #2557 fixed shadowing on the downloader `Fetch` path, where the
broker runs first. The stack-import resolver's eager pre-`Fetch` detect was never covered: the broker
simply hadn't run yet.

### 3. File mode (`git_config_mode: file`) is invisible to the shadowing guard

`brokerInsteadOfMatchesURL`/`gitConfigInsteadOfMatches` only matched inline `url.<base>.insteadOf`
keys. In file mode the broker instead emits `GIT_CONFIG_KEY_0=include.path` (the `insteadOf` rewrites
live *inside* the referenced gitconfig), so the guard never matched and ambient injection always won.
Narrow (only bites multi-owner + file mode — see analysis below), but a real latent gap.

### 4. Empty ref → `git checkout ""` in the getter's `update` path (unmasked by the auth fix)

Once auth worked, the clone surfaced a **pre-existing** bug in `pkg/downloader/get_git.go`
`CustomGitGetter`. `resolveGitSubdir` pre-creates the destination temp dir (`os.MkdirTemp`), so
go-getter's `os.Stat(dst)` succeeds and it takes the **`update`** path, not `clone`. `clone` defaults
an empty ref to the remote's default branch (`if ref == "" { ref = findRemoteDefaultBranch(...) }`);
**`update` did not**. The reproducer import has no `?ref=`, so `update` ran `git fetch -- ""` and
`git checkout ""` → `fatal: empty string is not a valid pathspec`. This was masked for years because
(a) every existing git-subdir test pins `?ref=main`, and (b) on this path the clone previously 404'd
on auth before ever reaching checkout. It also affects **ref-less public** git-subdir imports.

---

## Fix (PR #2568)

1. **Envelope** — `mint()` decodes via the new generic `pro.DecodeEnvelope[stsResponse]` and honors
   `success` (surfaces `EffectiveErrorMessage()` on a 200-with-`success:false`). New
   `pkg/pro/dtos/envelope.go` (`Envelope[T]` type, next to `AtmosApiResponse`) and
   `pkg/pro/envelope.go` (`DecodeEnvelope[T]`, next to the other Pro decoders) generalize the existing
   per-call shadow-`Data` pattern so every Pro response unwraps `data` through one sanctioned path.
2. **Timing** — `resolveGitSubdir` calls `broker.EnsureCredentials` **before** the eager
   `detectGitSource`, so the broker's `GIT_CONFIG_*` and `ATMOS_PRO_GITHUB_TOKEN` are live when the
   detector decides on injection. (`sync.Once`-guarded; gated to CI + configured.)
3. **File mode** — `gitConfigInsteadOfMatches` now also handles `include.path`: it resolves the
   referenced gitconfig and scans it for a matching **https** `insteadOf`
   (`fileInsteadOfMatches`/`insteadOfValueMatches`/`cutInsteadOfDirective`).
4. **Test seam** — `pkg/auth/broker/broker.go` adds `SwapRegistryForTest()` (mirrors
   `ci.SwapRegistryForTest`) so cross-package tests can register a fake broker in isolation.
5. **Empty-ref default** — `pkg/downloader/get_git.go` `update()` now defaults an empty ref to
   `findRemoteDefaultBranch(ctx, u)`, mirroring `clone()`, so a ref-less git-subdir import checks out
   the default branch instead of running `git checkout ""`.

---

## What we learned: token bridge vs. git-config (the important part)

The github/sts integration, in the **default** env mode, *always* exports **two** carriers of the
same minted token from `Environment()`:

1. **Per-owner git `insteadOf`** via `GIT_CONFIG_*` (env mode) or an `include.path` gitconfig (file
   mode). `git_config_mode` only controls *how* it is materialized.
2. **The raw token** under `token_env`, defaulting to `ATMOS_PRO_GITHUB_TOKEN`.

These are **not** an either/or — both ship by default, even though a user who set only `repos:`
never asked for git-config. `CustomGitDetector.resolveToken` already prefers
`ATMOS_PRO_GITHUB_TOKEN` (struct field → **live env**, the latter added by #2557) over the ambient
`GITHUB_TOKEN`. So for the common case the raw token alone is sufficient and flows through the
**existing** token-injection path with zero git-config involvement.

**So why does git-config exist, and is emitting it the right behavior?** It is the more *general*
mechanism, needed for exactly two cases the bare token cannot cover — both verified in code:

- **ssh/scp imports.** Token injection is **https-only**. `needsTokenInjection` returns `false` for
  `ssh://git@github.com/…` and for scp-style `git@github.com:owner/repo` (rewritten to `ssh://git@…`),
  because the `git@` user is already present — confirmed by
  `token_injection_helpers_test.go` ("SSH URL with git@ user does not need injection"). The prior
  `GITHUB_TOKEN` work also never token-authenticated ssh refs. Only the git-config `insteadOf`
  ssh-variant (`ssh://git@github.com/owner/` → https-with-token) bridges ssh to a token.
- **Multi-owner mints.** Atmos Pro returns **one token per GitHub App installation** (≈ per
  owner/org); `stsResponse.Tokens` is a slice and the contract is "expect multiple." A single
  `ATMOS_PRO_GITHUB_TOKEN` env var cannot carry several owners' tokens, and `resolveToken` only knows
  that one var — so per-owner `insteadOf` is the only in-process way to route a multi-owner mint.
  `addTokenEnv` deliberately **skips** the raw export for multi-owner literal `token_env`.

For the reproducer config — **single owner, https** (`github.com/cloudposse-sandbox/…`, scheme-less
→ https) — **neither** ssh nor multi-owner applies. The token bridge is fully sufficient; git-config
buys nothing here.

The wrinkle that turned "easy token" into "convoluted git-config": #2557's shadowing guard makes
`injectToken` **skip** URL injection whenever *any* broker `insteadOf` matches — including the
single-owner case where the brokered `ATMOS_PRO_GITHUB_TOKEN` is the correct, available credential.
So even after the timing fix, the clone resolves via git-config, and the simple token bridge the user
configured is silently bypassed. That skip is correct for **multi-owner** (where only the ambient
token would otherwise be injected) but over-broad for **single-owner**.

---

## Recommended follow-up (NOT in this PR)

Make the simple token bridge the mechanism for the common case, keeping git-config as the fallback it
was meant to be. In `CustomGitDetector.injectToken`:

```go
token, source := d.resolveToken(host)
brokered := source == "ATMOS_PRO_GITHUB_TOKEN" && token != ""
// Skip only when we'd otherwise inject an *ambient* token over a broker rewrite (multi-owner).
if !brokered && brokerInsteadOfMatchesURL(parsedURL, host) {
    return
}
if token != "" {
    // inject token (https)
}
```

Effect: single-owner https imports (the overwhelmingly common case) resolve via the injected
`ATMOS_PRO_GITHUB_TOKEN` — the documented bridge, identical to how `GITHUB_TOKEN` already works —
while multi-owner and ssh keep using git-config. This removes the git-config dependency for the case
users actually hit, without losing the coverage it uniquely provides.

Status: discussed, not implemented. Track separately before adding more callers to the skip path.

---

## Tests

- `pkg/pro/envelope_test.go` — unwraps nested `data`; **canary**: a flat `{tokens}` body decodes to
  empty `Data` (documents why the envelope is mandatory); `success=false` surfaces the message;
  generic over payload type; malformed JSON errors.
- `pkg/auth/integrations/github/sts_test.go` — `TestGitHubSTSMint_DecodesCanonicalEnvelope` serves the
  exact server wire payload and asserts 1 token persisted (not 0); the `stsServer` helper was fixed to
  emit the real envelope (it previously emitted the unwrapped shape the server never sends).
- `pkg/stack/imports/remote_broker_test.go` — registers a fake broker (via `SwapRegistryForTest`) +
  mock downloader; asserts the detected source URL carries no ambient userinfo. Verified to **fail**
  against the unfixed code with the exact symptom (`x-access-token:ambient-wrong@github.com/...`).
- `pkg/downloader/broker_insteadof_filemode_test.go` — file-mode `include.path`: matched owner not
  shadowed, different owner/host still injected, ssh-only and missing-file negative cases.
- `pkg/stack/imports/remote_test.go` — `TestRemoteImporter_Resolve_GitSubdir_NoRef_DefaultsToBranch`:
  a ref-less git-subdir import resolves via the default branch. Verified to **fail** against the
  unfixed `update` with the exact `empty string is not a valid pathspec` error. (`initGitRepo` also
  now disables `commit.gpgsign` so the real-git fixture is deterministic on dev machines with signing
  enabled.)

---

## Verification

```bash
go build ./...
go test ./pkg/pro/... ./pkg/auth/integrations/github/... ./pkg/stack/imports/... ./pkg/downloader/... ./pkg/auth/broker/...
```

End-to-end (env mode, single owner): after the timing fix the broker runs before the eager detect, so
the minted token reaches the in-process git path and the cross-repo `import:` resolves. Final
confirmation requires a CI run of the `-xl` scenario against this build.

---

## Related

- `docs/fixes/2026-06-01-github-sts-token-injection-shadowing.md` — #2557, the Fetch-path shadowing
  fix this builds on
- `docs/prd/atmos-pro-sts.md` — STS PRD (mint contract, `token_env`, shadow-avoidance)
- PR #2546 (introduced `github/sts`), PR #2568 (this fix)
- `pkg/auth/integrations/github/sts.go` — `mint()`, `environmentEnvMode`/`environmentFileMode`, `addTokenEnv`
- `pkg/downloader/custom_git_detector.go` — `injectToken`, `resolveToken`, `brokerInsteadOfMatchesURL`
- `pkg/stack/imports/remote.go` — `resolveGitSubdir`, `detectGitSource`
- `pkg/pro/envelope.go`, `pkg/pro/dtos/envelope.go` — the canonical envelope decoder + type
