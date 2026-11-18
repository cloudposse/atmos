# saml2aws (Atmos fork)

This is Atmos's vendored fork of the library parts of [saml2aws](https://github.com/Versent/saml2aws)
v2.36.19. The `aws/saml` auth provider (`pkg/auth/providers/aws/saml.go`) uses it to sign in to a SAML
identity provider and read the AWS roles from the SAML assertion.

## Why it is vendored

- Upstream is no longer maintained. The last release was v2.36.19 (March 2025), the last commit was in
  May 2025, and [Versent/saml2aws#1545](https://github.com/Versent/saml2aws/issues/1545) ("Still maintained?") has no reply.
- Upstream pins `github.com/playwright-community/playwright-go` v0.5700.1, the last release under that module path.
  It pulls in `github.com/go-jose/go-jose/v3`, which has an unfixed HIGH advisory (CVE-2026-116587 / GHSA-mgcc-cww5-fp2q)
  that vulnerability scanners report against the Atmos binary.
- The upstream browser provider panics when the sign-in times out.

## What was kept

| Path | Upstream path | Purpose |
|------|---------------|---------|
| `saml2aws.go`, `saml.go`, `aws_role.go` | repo root | `NewSAMLClient` dispatcher, assertion parsing, role parsing |
| `cfg/` | `pkg/cfg` | `IDPAccount` |
| `creds/` | `pkg/creds` | `LoginDetails` |
| `provider/` | `pkg/provider` | Shared HTTP client |
| `provider/browser/` | `pkg/provider/browser` | `Browser` driver (Playwright) |
| `provider/googleapps/` | `pkg/provider/googleapps` | `GoogleApps` driver |
| `provider/okta/` | `pkg/provider/okta` | `Okta` driver |
| `provider/adfs/` | `pkg/provider/adfs` | `ADFS` driver |
| `prompter/`, `page/`, `cookiejar/`, `dump/`, `helper/credentials/` | same names under `pkg/` or `helper/` | Shared helpers used by the drivers |
| `mocks/` | `mocks` | Upstream mockery mocks used by the copied Okta and GoogleApps tests |

## Changes from upstream

- **Drivers:** only `Browser`, `GoogleApps`, `Okta` and `ADFS` are kept, which are the drivers the Atmos docs list.
  Any other driver fails with `ErrUnsupportedProvider`, and the Atmos provider rejects it when that provider logs in
  (not during config validation, so one stale provider does not break other identities).
- **Playwright:** the browser driver uses `github.com/mxschmitt/playwright-go` v0.6401.0 (the project's current
  module path). That release no longer depends on `go-jose`.
- **Browser driver fixes** (`provider/browser/browser.go`):
  - A timeout or error from `ExpectRequest` no longer dereferences a nil request (panic).
  - A `PostData` error is returned instead of being swallowed by returning the wrong variable.
  - The request callback and the caller no longer race on the captured data.
  - An empty capture returns `ErrNoSAMLResponse` instead of an empty assertion.
  - A sign-in request that reaches the `OnRequest` callback just before `ExpectRequest` starts listening is
    used instead of waiting for the full timeout and failing.
  - An invalid `browser_type` is rejected before Playwright is installed or started.
  - The Playwright driver, the browser and the browser context are each released as soon as a later step
    fails. Upstream released them only after the page was created, so a failed launch leaked the driver process.
  - `playwright install` writes its progress to stderr instead of stdout, and installs only the browser that
    is launched (Chromium without the headless shell for a headed browser, or Firefox or WebKit) instead of
    every Playwright browser. System browser channels (`chrome`, `msedge`, ...) keep upstream's behavior.
  - The storage-state path is built with `filepath.Join` and the Atmos `homedir` package, and its parent
    directory is created with `0700` permissions before saving.
- **`cfg`:** the ini-file `ConfigManager` (`~/.saml2aws`) was removed. Atmos builds `IDPAccount` from `atmos.yaml`.
  The unused `IDPAccount.DownloadBrowser` field was removed: nothing in the library read it (only the saml2aws CLI
  did, through `SAML2AWS_AUTO_BROWSER_DOWNLOAD`). The browser driver installs browsers when
  `creds.LoginDetails.DownloadBrowser` is set.
- **GoogleApps driver fixes** (`provider/googleapps/`):
  - The captcha loop used the legacy `.captcha-img` container only as dead code (a `*goquery.Selection` is never
    nil), so it always looked for `div[data-auto-init='CaptchaInput']`. The legacy container is used when present,
    the new one is the fallback, and a missing or empty container returns an error instead of panicking.
  - `mustFindInputByName` called `logger.Fatal` (process exit) for an input without a value. It is now
    `findInputByName` and returns an error.
  - A rejected new-style captcha answer is asked again. Upstream re-checked only `#ca`, but that input is
    `name="ca"` with no id, so the flow gave up and failed. Captcha retries are capped at 5.
  - The iTerm inline captcha image is written to stderr instead of stdout.
  - `b64Safe` returns an error instead of panicking on invalid base64, and the U2F device is closed on that error.
- **Okta driver:** the Duo U2F touch prompts are written to stderr instead of stdout. The Duo trusted-endpoint
  check skips certificate verification only when `certifier_url` is a loopback host (the local certifier uses a
  self-signed certificate); upstream skipped it for any URL the Duo page named.
- **TLS:** every IdP transport (`provider.NewDefaultTransport`, ADFS, the Okta Duo certifier) sets
  `MinVersion: tls.VersionTLS12` (`provider.MinTLSVersion`). TLS 1.3 is not required because many enterprise IdPs,
  notably ADFS, still serve TLS 1.2 only, and ADFS relies on renegotiation, which TLS 1.3 removed.
- **ADFS driver:** unnamed inputs and a `SAMLResponse` without a value no longer call `log.Fatalf`, and the MFA loop
  is limited: at most 5 one-time codes and 300 Azure MFA approval polls, then `ErrMFAAttemptsExceeded` is returned
  instead of looping forever.
- **`provider/http.go`:** an `http_attempts_count` of `0` is clamped to one attempt (retry-go v3 ran nothing and
  returned an empty error), invalid attempt and delay values are logged at debug level, and a client created with
  nil options sends the request once instead of panicking.
- **Later review fixes:**
  - `aws_role.go` keeps commas inside role names (a comma separates ARNs only before another `arn:`).
  - `saml.go` matches the Attribute elements by namespace URI instead of the Assertion's prefix.
  - `cookiejar` no longer decodes the default cookie path twice.
  - `page.Form` puts GET form fields in the URL query, and no longer panics when the document has no URL.
  - The `prompter` methods return errors (Ctrl+C or a missing terminal stops authentication instead of choosing
    the first option or sending an empty code), pinentry returns scanner errors, and selecting a default alias
    resets an earlier pinentry choice.
  - `provider/http.go` resends the request body on retries, binds retry waits to the request context, and closes
    the body of a response rejected by the status validator.
  - ADFS resolves every relative form action against the login page URL.
  - Browser: the storage state is saved through a private temporary file, and a captured SAMLResponse is not
    replaced by a later request without one.
  - Google Apps: locale parameters are set in the query, challenge selection pages are followed at most 10 times
    per attempt, the MFA response document has a URL, headings are matched with whitespace normalized, and
    malformed `data-challenge-ui` values no longer panic.
  - Okta: both MFA polling loops stop after `mfaPollTimeout`, a failed Duo health report cancels the status request,
    the Duo callback response is closed, and the next FIDO device is searched for with the `Auto` setting.
- **Tests:** browser tests use `go.uber.org/mock` mocks of a narrow `samlPage` interface instead of a real browser.
  Real-browser tests are in `browser_integration_test.go` and only run with `RUN_PLAYWRIGHT_INTEGRATION=1`.

## Updating

Upstream is unmaintained, so there is nothing to sync. Treat this package as Atmos code: fix bugs here and add tests.
Keep the `Atmos fork:` comments when changing upstream behavior so the differences stay easy to find.
