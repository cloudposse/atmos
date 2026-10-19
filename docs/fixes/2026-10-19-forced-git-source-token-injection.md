# Fix: GitHub/GitLab/Bitbucket token injection skipped for "git::"-forced, schemed sources

**Date:** 2026-10-19

## Summary

Token injection for git-clone sources (GitHub, GitLab, Bitbucket) was silently skipped whenever
the source was written as an explicitly-schemed, forced git source, e.g.
`git::https://github.com/org/repo.git`. Only scheme-less shorthand sources (e.g.
`github.com/org/repo`) reliably got a token injected. Since both forms are common and
interchangeable in stack manifest imports, vendor sources, and scaffold sources, this looked like
inconsistent/"sometimes skipped" token injection depending on which form a given source happened
to use, rather than a reproducible bug.

## Context

- `pkg/downloader/custom_git_detector.go`'s `CustomGitDetector.Detect()` is the component that
  actually injects a GitHub/GitLab/Bitbucket token into a git source URL. It is registered as a
  `getter.Detector` and normally runs via go-getter's own package-level `Detect()` function
  (`detect.go` in the vendored `go-getter` module), which `getter.Client.Get()` calls before
  dispatching to a getter.
- Go-getter's own `Detect()` has an early return: once the (force-prefix- and subdir-stripped)
  source already parses with a non-empty URL scheme, it returns immediately WITHOUT calling any
  registered `Detector` at all -- `CustomGitDetector` never runs. This only lets
  scheme-less shorthand sources (`github.com/org/repo`) reach the detector through go-getter's
  normal dispatch; both a plain `https://github.com/org/repo.git` and an explicitly-forced
  `git::https://github.com/org/repo.git` source skip it entirely.
- `pkg/downloader/gogetter_downloader.go` already had a comment documenting half of this gap and
  a partial fix: `TestNewClient_AttachesGitHubTokenToHTTPGetter` / the `httpGetter.Client` wiring
  in `goGetterClientFactory.NewClient` attaches a GitHub token directly to the `http`/`https`
  getter for explicitly-schemed sources that end up routed there (e.g.
  `https://raw.githubusercontent.com/...`). That fix never covered the git-clone side: a
  `git::`-forced, already-schemed source routed to the `git` getter (`CustomGitGetter`) had no
  equivalent, since `CustomGitGetter` itself does no URL-level token injection -- it relies
  entirely on `CustomGitDetector` having already embedded credentials into the URL beforehand.
- `pkg/downloader/token_injection_e2e_test.go`'s `TestCustomGitDetector_EndToEnd_GitHubTokenFallback`
  (despite its "EndToEnd" name) calls `CustomGitDetector.Detect()` directly, including a
  `"GITHUB_TOKEN with git:: prefix"` case -- so the detector itself was always proven correct in
  isolation. It never went through the real dispatch path (`getter.Client.Get()` -> go-getter's own
  `Detect()` wrapper), which is exactly the layer that silently skips the detector for an
  already-schemed source. This is why the gap existed without a failing test to surface it.

## Changes

- `pkg/downloader/gogetter_downloader.go`: added `applyForcedGitDetection(ctx, atmosConfig, src)`,
  called at the top of `goGetterClientFactory.NewClient` before constructing the `getter.Client`.
  It re-runs `CustomGitDetector.Detect()` directly against the inner URL of a `"git::"`-forced
  source whenever that inner URL already carries an explicit scheme, bypassing go-getter's own
  early-return. It is a no-op for:
  - Sources without a `"git::"` prefix (scheme-less shorthand already reaches the detector through
    go-getter's normal dispatch; nothing to fix there).
  - A `"git::"`-forced source whose inner URL has no explicit scheme (same reason).
  - Hosts `CustomGitDetector` doesn't recognize (anything other than GitHub, a configured GitHub
    Enterprise Server host, GitLab, or Bitbucket): `Detect()` reports no match and the source is
    left unchanged.

  For a supported host it is not a no-op even when no token is injected (injection disabled in
  settings, no token configured, credentials already present in the URL, or a non-https scheme).
  `Detect()` still matches, so the source is rewritten the same way a scheme-less source already is:
  it keeps its `git::` prefix, has its path normalized, and gets `depth=1` added when no `depth` is
  set. Only the credential step is skipped in those cases. The token-injection decision itself
  (`shouldInjectTokenForHost`, `injectToken`) is `CustomGitDetector`'s existing logic and is
  unchanged.
- `pkg/downloader/gogetter_downloader_test.go`: added
  `TestNewClient_InjectsTokenForForcedGitSource` (proves a `git::https://github.com/...` source
  now gets a token injected into `ggc.client.Src`) and
  `TestNewClient_DoesNotForceGitDetectionForPlainHTTPSource` (proves a plain, non-forced
  `https://github.com/.../archive/...` source -- e.g. a GitHub archive/release download routed to
  the `http`/`https` getter -- is left untouched, since reinterpreting it as a git clone target
  would break that existing, intentional use case).

## Validation

- `go build ./pkg/downloader/...` and `go vet ./pkg/downloader/...` -- clean.
- `go test ./pkg/downloader/...` -- all tests pass, including the two new regression tests and
  every pre-existing token-injection/detector test (`token_injection_e2e_test.go`,
  `token_injection_test.go`, `custom_git_detector_test.go`, `broker_insteadof_*_test.go`, etc.).
- `go test ./pkg/vendoring/... ./pkg/stack/imports/... ./pkg/generator/source/... ./pkg/provisioner/source/...`
  -- all pass, confirming no regression in the consumers of the shared go-getter downloader whose
  sources could include a `git::`-forced form.

## Follow-ups

None.
