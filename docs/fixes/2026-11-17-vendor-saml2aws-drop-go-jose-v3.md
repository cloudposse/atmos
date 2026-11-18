# Fix: Vendor `saml2aws` to drop `go-jose/v3` and fix the SAML browser driver

**Date:** 2026-11-17

## Summary

The `aws/saml` auth provider depended on `github.com/versent/saml2aws/v2` v2.36.19. That library is no longer
maintained, and through `github.com/playwright-community/playwright-go` v0.5700.1 it pulled
`github.com/go-jose/go-jose/v3` into the Atmos binary. go-jose v3 has an unfixed HIGH advisory
(CVE-2026-116587 / GHSA-mgcc-cww5-fp2q, CPU denial of service when parsing JWKs with RSA keys), so
vulnerability scanners reported it against Atmos.

The library parts Atmos uses are now vendored (MIT, with attribution) into
`pkg/auth/providers/aws/saml2aws/`. The copy uses the current Playwright module, so `go-jose/v3` is no longer in
the module graph or the binary. The copy also fixes the browser driver's timeout panic and related bugs.

## Context

- **Upstream status:** the last saml2aws release was v2.36.19 (2025-03-13) and the last commit was on 2025-05-13.
  The repo has 300 open issues and 66 open PRs, and
  [Versent/saml2aws#1545](https://github.com/Versent/saml2aws/issues/1545) ("Still maintained?") has no reply.
- **Why a version bump could not fix it:** after v0.5700, playwright-go moved back to the
  `github.com/mxschmitt/playwright-go` module path, and v0.6401.0 dropped go-jose (it uses `encoding/json`).
  saml2aws still imports the old `playwright-community` path, and v0.5700.1 is the last release under it.
  A `replace` from the old path to `mxschmitt/playwright-go` does not build, because the new module imports its own
  `internal/` packages and Go does not allow that under an aliased path.
- **Reachability before the fix:** only `go-jose/v3/json` was linked, not the vulnerable JWK code. Module-level
  scanners still flagged it, and arguing "not reachable" with each scanner was not a durable answer.
- **The other CVE in the same scan** (CVE-2026-116591, `klauspost/compress/zstd` <= 1.20.1) was already fixed on
  `main` by #3688 (bump to 1.20.2). It is not part of this change.

## Changes

**Vendored package `pkg/auth/providers/aws/saml2aws/`** (see its `README.md` and `ATTRIBUTIONS.md`; upstream
`LICENSE.md` kept):

- **Kept:** the root package (`NewSAMLClient`, assertion and role parsing), `cfg`, `creds`, the shared `provider`
  HTTP client, the `Browser`, `GoogleApps`, `Okta` and `ADFS` drivers, and the helpers they use (`prompter`,
  `page`, `cookiejar`, `dump`, `helper/credentials`).
- **Dropped:** the 15 IdP drivers Atmos never documented (AzureAD, ADFS2, Ping, PingNTLM, PingOne, JumpCloud,
  OneLogin, Authentik, KeyCloak, Shibboleth, ShibbolethECP, F5APM, Akamai, Shell, NetIQ, Auth0), plus the
  ini-file `ConfigManager` (`~/.saml2aws`), which Atmos never used. `NewSAMLClient` returns
  `ErrUnsupportedProvider` with the list of supported drivers for any other value.
- **Playwright:** the browser driver moves to `github.com/mxschmitt/playwright-go` v0.6401.0. Its
  `StorageState` call is updated for the new API.
- **Browser driver bug fixes** (each has a regression test):
  - A timeout or error from `ExpectRequest` no longer dereferences a nil request. This was the panic that
    `saml.go` previously documented as an upstream bug with no fix.
  - A `PostData` error is returned instead of being swallowed. Upstream returned the wrong error variable,
    which could report success with an empty assertion.
  - The request callback and the waiting code no longer race on the captured data.
  - An empty capture returns `ErrNoSAMLResponse` instead of an empty assertion.
  - An invalid `browser_type` is rejected before Playwright is installed or started.
  - A sign-in request that reached the `OnRequest` callback just before `ExpectRequest` started listening was
    missed, so the user waited the full 5-minute timeout and failed. It is now used.
  - The Playwright driver, browser and context are each released as soon as a later step fails. Upstream
    leaked the node driver process when the browser failed to launch.
  - `playwright install` progress goes to stderr instead of stdout, where it corrupted `eval $(atmos auth env)`.
    Only the browser that will be launched is installed, not every Playwright browser.
  - The storage-state path is built with `filepath.Join` and `homedir.Dir()` instead of a forward-slash
    `fmt.Sprintf`. Its directory is created with `0700` permissions, and the file, which holds IdP session cookies
    and which Playwright writes as `0644`, is tightened to `0600` after saving.
- **Other upstream bugs found while adding tests** (each marked `// Atmos fork:` with a regression test):
  - ADFS: `checkResponse` called `log.Fatalf`, which exits the whole process, on an unnamed `<input>` or a
    `SAMLResponse` input with no value. It now skips unnamed inputs and returns an error.
  - Prompter: the pinentry runner raced on its error result and closed the output pipe before all of it was
    read. It now runs `Start`, then parses the output, then calls `Wait`.
  - Okta:
    - A Duo `FAILURE` and a missing Duo `sid` returned `errors.Wrap(nil, ...)`, which is `nil`, so they were
      reported as success.
    - FIDO fallback to the next key after a bad key handle never ran, because the search restarted at the
      option that had just failed.
    - Nil-response panics on a transport error with an existing session, and in the Duo endpoint-health
      request.
    - An index panic on a malformed Duo signature.
  - GoogleApps:
    - `challenge/skotp` URLs were handled as `challenge/sk` (U2F) because of the case order.
    - Index panics on a form with no `action` and on a U2F page with no key handles.
    - The captcha loop never used the legacy `.captcha-img` container (a `*goquery.Selection` is never nil) and
      panicked on an empty container. A rejected new-style captcha answer was never retried, because the input is
      `name="ca"` with no id. Retries are capped at 5.
    - `mustFindInputByName` called `logger.Fatal` (process exit); `b64Safe` panicked on invalid base64. Both now
      return errors.
  - ADFS: the MFA loop had no limit. It now allows at most 5 one-time codes and about 5 minutes of Azure MFA
    polling, then returns `ErrMFAAttemptsExceeded`.
  - Shared HTTP client: `HttpAttemptsCount: "0"` (or an invalid value) made every request fail without being sent.
    Attempts are clamped to at least 1, and a nil options value no longer panics.
  - stdout: the iTerm captcha image (GoogleApps) and the Duo U2F touch prompts (Okta) were written to stdout, the
    data channel, which corrupts output such as `eval $(atmos auth env)`. They go to stderr.
  - TLS (code-scanning `missing-ssl-minversion`): every IdP transport (shared HTTP client, ADFS, Okta Duo
    certifier) now sets an explicit TLS 1.2 minimum. TLS 1.3 is not required because many enterprise IdPs,
    notably ADFS, still serve TLS 1.2 only, and ADFS relies on renegotiation, which TLS 1.3 removed.
  - Okta Duo trusted-endpoint check: upstream skipped certificate verification for whatever `certifier_url` the
    Duo page named. Verification is now skipped only for a loopback host, where the local certifier runs with a
    self-signed certificate.
- **Testability:** `getSAMLResponse` takes a narrow `samlPage` interface, mocked with `go.uber.org/mock`.
  The poll and settle delays in Okta and ADFS are package variables, so tests do not sleep.
  Tests that need a real browser moved to `browser_integration_test.go` and only run with
  `RUN_PLAYWRIGHT_INTEGRATION=1`.

**Atmos `aws/saml` provider (`pkg/auth/providers/aws/`):**

- `saml.go` imports the vendored package. `Authenticate` rejects an unsupported `driver` value first, with
  `ErrInvalidProviderConfig` and the list of supported drivers. The check is deliberately not in `Validate()`:
  the auth manager validates every configured provider up front, so a stale driver on one unused SAML provider
  would otherwise break authentication for every identity.
- **New `saml_browser_revision.go`, for a regression the Playwright bump would otherwise cause.** The old
  "drivers installed" check accepted any non-empty browser directory. After the upgrade, a user with Chromium
  installed for the old driver would have auto-download turned off, and the new driver would fail to launch.
  `shouldDownloadBrowser` now checks for the exact revision of the browser that will be launched (Chromium by
  default, or `browser_type: firefox|webkit`), named in the driver's `package/browsers.json`, including
  per-platform `revisionOverrides`. It requires Playwright's `INSTALLATION_COMPLETE` marker so an interrupted
  download does not count. Playwright-managed `browser_type` values (`chromium`, `firefox`, `webkit`) no longer
  turn auto-download off; only system channels (`chrome*`, `msedge*`) and `browser_executable_path` do. It looks only where the driver itself looks: `PLAYWRIGHT_BROWSERS_PATH` (including `0`),
  then `$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, and `%LOCALAPPDATA%` on Windows.
  `getDriver` still uses the broader check, so users who rely on auto-detection are not silently switched from
  the `Browser` driver to an API driver.
- `saml_driver_preseed.go`: switched to the new Playwright module. It is kept because, unlike playwright-go's own
  download, it verifies the npm tarball's `sha512` integrity. After pre-seeding it now applies playwright-go's
  own `coreBundle.js` patch, which v0.6401 only applies on its own download path, so a pre-seeded driver no
  longer runs unpatched.
- `SAML2AWS_AUTO_BROWSER_DOWNLOAD` was dead: nothing read it, but `setupBrowserAutomation` set or unset it
  process-wide (clobbering a value the user exported) and `Environment()` exported it to subprocesses. Both are
  removed, along with the unused `IDPAccount.DownloadBrowser` field, and the two auth PRDs that claimed exporting
  it forces a download are corrected. The download decision reaches the driver through `LoginDetails`.
- Comments that described the upstream panic and the hardcoded upstream storage path were updated.

**Repository:**

- `go.mod`/`go.sum`: removed `github.com/versent/saml2aws/v2`, `github.com/playwright-community/playwright-go`
  and `github.com/go-jose/go-jose/v3`. `github.com/mxschmitt/playwright-go` moved from v0.6100.0 to v0.6401.0.
- `NOTICE`: regenerated. It now lists the vendored package through its `LICENSE.md`, and drops the removed modules
  and `go-ntlmssp`, which only a dropped driver used.
- `.gitignore`: the repo-wide `credentials` rule ignored the vendored `helper/credentials` package, so the first
  push of this change was missing it and CI could not build. It is re-included with an exact-path exception, the
  same way `pkg/auth/credentials/` already is.
- `.golangci.yml`: Atmos style and convention linters are relaxed for the vendored path. Correctness linters
  still apply.
- `errors/errors.go`: added `ErrPlaywrightBrowserManifest`.
- Docs: `website/docs/cli/configuration/auth/providers.mdx` (an unsupported `driver` fails at login, and names
  are case-sensitive) and `website/docs/cli/commands/auth/usage.mdx` (removed the reference to saml2aws).

**Behavior change:** an `aws/saml` provider that sets `driver` to one of the 15 undocumented values now fails
with a clear error when that provider logs in, instead of working. Other providers and identities are not
affected. The documented drivers are unchanged, and `Browser` works with any IdP that ends
on the AWS SAML sign-in page.

## Validation

- `go build ./...`: passes.
- `go list -deps ./` (the `atmos` binary): no `go-jose/v3`, `playwright-community` or `versent` packages.
  `go.mod` no longer lists `go-jose/v3`.
- `go vet ./pkg/auth/providers/aws/...`: passes.
- `go test -count=1 ./pkg/auth/providers/aws/`: passes, including the new driver-validation and
  Chromium-revision tests and the updated tests that create fake browser installs.
- `go test -race -count=1 -cover ./pkg/auth/providers/aws/...`: passes. Coverage of the vendored packages:
  - 100%: root, `cfg`, `dump`, `helper/credentials`, `page`
  - `provider` 97.2%, `prompter` 96.6%, `provider/adfs` 95.8%, `cookiejar` 93.8%
  - `provider/googleapps` 89.5%, `provider/okta` 86.8%
  - `provider/browser` 69.1%. The rest needs a real browser and is covered by the opt-in integration tests.
  - The Atmos `aws` provider package is at 84.0%.
- `go test -race -count=1 ./pkg/auth/... ./cmd/auth/... ./pkg/plugin/adapter/auth/...`: all 61 packages pass,
  including the tests added during the pre-release auth review. `go vet` passes for linux, windows and darwin.
- `go mod tidy -diff`: clean.
- `go tool mage notice:generate`: regenerated `NOTICE` (CI checks it).
- `cd website && npm run build`: passes.
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh` on this file: passes.
- **Not run:**
  - `golangci-lint`, `gofmt` and `gofumpt` were not allowed in this session. Imports were regrouped by hand into
    stdlib / third-party / atmos groups, but lint and format still need a run locally or in CI.
  - Real-browser tests (`RUN_PLAYWRIGHT_INTEGRATION=1`) were not run because they download Chromium.
  - No interactive SAML login against a real IdP was done.

## Follow-ups

- Before merging, run `atmos lint --changed` and do one interactive `aws/saml` login with the `Browser` driver.
  Ideally the cache should hold an old Chromium revision, to confirm the auto-download.
- A pre-release review of the rest of Atmos Auth (code already merged to `main`, outside this change) found
  bugs that are tracked in #3720 and will be fixed in a separate branch. Four of them are regressions new
  since v1.234.1.
- None of the known issues in the code this change touches are left open: the upstream quirks found during review
  (ADFS MFA loop, `logger.Fatal`, captcha selection and retry, `HttpAttemptsCount: "0"`) are fixed above.
