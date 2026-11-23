# Fix: Plugin sign-in is printed verbatim and bounded separately from the package timeout

**Date:** 2026-11-22

## Summary

Two problems in native auth plugins, both reported in the Atmos Auth pre-release review (#3720):

- The device-code verification URL was rendered as Markdown, so a URL such as `.../a*b*c/device` was
  shown as `.../abc/device` (finding 1.4, listed in #3720 among the regressions new since v1.234.1).
- `plugins.packages.<alias>.timeout` counted the time the user spent signing in. A short timeout made
  device-code and browser login impossible (a suspected finding in #3720, confirmed as real).

The prompt is now plain text. In an interactive call, the timeout counts only the plugin's own work. The
user's sign-in wait has its own limit, configured by a new per-package key,
`plugins.packages.<alias>.interaction_timeout` (default 15m, at most 24h, requires `timeout`). The
interactive hard deadline is `timeout` plus that limit, and it no longer overflows when `timeout` is very
large.

This PR adds a new configuration key, so it is a feature change (`minor`), not only a bug fix. Other
#3720 and regression-test fixes land in separate PRs.

## Context

A native plugin provider or identity can send one `device_code` interaction before its credential result.
The host validates the verification URI and user code, prints them on the UI channel (stderr), and waits
for the plugin to finish. The plugin polls the identity provider until the user approves the sign-in.
Plugins can also use the host Browser API, whose `Listen` call waits for the OAuth redirect callback.

Before this change, the native runtime applied `timeout` as one fixed context deadline to the whole
invocation (`native.WithCallTimeout`). Nothing in the runtime knew when the plugin was waiting for a
person, so the sign-in wait and the plugin's own work shared one budget.

## Changes

### 1. Device-code URL printed verbatim

- Problem: a valid verification URI that contains Markdown characters (`*`, `_`, backticks, `[..](..)`)
  was shown to the user as a different URL.
- Cause: `presentDeviceCode` used `ui.Infof`, which renders Markdown.
- Fix: the prompt is written with `ui.Writef("Open %s and enter code %s.\n", ...)` as plain text on the UI
  channel. A comment records why it must not use a Markdown helper.
- Files: `pkg/plugin/adapter/auth/interactions.go`.
- Docs: `website/docs/cli/configuration/plugins.mdx` now says the URL and code are written as plain text,
  exactly as the plugin sent them.
- Test: `TestPresentDeviceCodePrintsVerificationURIVerbatim` (`a*b*c`, `a_b_c_`, backticks and link
  syntax all appear unchanged on stderr, and stdout stays empty).

### 2. The user's sign-in wait does not count against `timeout`

- Problem: with `timeout: 5m`, a user who took more than 5 minutes to approve a device-code or browser
  sign-in had the plugin killed mid-login.
- Cause: the call timeout was one fixed deadline over the whole invocation.
- Fix: interactive invocations with a call timeout get a pausable work budget
  (`pkg/plugin/runtime/native/interaction.go`).
  - The budget equals `timeout`. When the plugin has worked that long outside user waits, it cancels the
    call with cause `ErrPluginTimeout`, so a plugin that hangs without prompting is still stopped.
  - `Handle.AwaitUser(invocationID)` pauses the budget and returns an idempotent resume function. Waits
    nest; the budget runs again only when every wait has ended. Unknown invocations and calls without a
    timeout get a no-op.
  - The auth adapter opens a wait right after it shows the device prompt and ends it when the credential
    result arrives. Progress events do not end the wait. A stream error also ends it
    (`receiveCredentials` in `pkg/plugin/adapter/auth/invoke.go`, used by `provider.go` and
    `identity.go`).
  - The bridge's Browser `Listen` pauses the budget only while it waits for the sign-in callback
    (`pkg/plugin/bridge/browser.go`). `bridge.Invocation` gained an `AwaitUser` hook that the runtime sets
    for interactive calls with a timeout (`pkg/plugin/bridge/invocations.go`).
  - `callContext` now uses `context.WithCancelCause`. The hard deadline of an interactive call is
    `timeout` plus the interaction limit, and that is the deadline the plugin sees in its invocation and
    RPC context (`pkg/plugin/runtime/native/lifecycle.go`, `native.go`).
  - `finishInvocation` reports three cases: the work budget ran out (generic timeout error); the call's
    own hard deadline passed (a new error whose hint says Atmos waited the limit for the sign-in and names
    `plugins.packages.<alias>.interaction_timeout`); or an earlier deadline set by the caller passed
    (generic timeout hint).
  - The budget also counts the cumulative time spent in user waits against the interaction limit. When
    those waits add up to the limit, the call is stopped with the sign-in error, even when `timeout` is
    far larger, so the wait never exceeds `interaction_timeout` (review follow-up). A budget whose work
    allowance is already spent when a wait opens expires at once instead of pausing.
  - Noninteractive invocations cannot prompt, so they keep the fixed `timeout` deadline.
- Tests:
  - Budget unit tests in `pkg/plugin/runtime/native/interaction_test.go`:
    `TestBudgetExpiresAfterTimeoutWithoutUserWait`, `TestBudgetDoesNotCountUserWait`,
    `TestBudgetStaysPausedUntilEveryWaitEnds`, `TestBudgetPausedAtExpiryRunsOutWhenResumed`,
    `TestBudgetUsedUpBeforeWaitExpiresInsteadOfPausing`, `TestBudgetWaitIsBoundedByWaitLimitDespiteLargeTimeout`,
    `TestBudgetWaitLimitAccumulatesAcrossWaits`, `TestStoppedBudgetNeverExpires`, `TestAwaitUserIgnoresUnknownInvocation`.
  - Runtime behavior in the same file: `TestInteractiveCallTimeoutStopsHungPluginBeforeHardDeadline`,
    `TestInteractiveUserWaitIsBoundedByInteractionLimit`,
    `TestInteractiveCallerDeadlineKeepsGenericTimeoutHint`, `TestNoninteractiveCallKeepsCallTimeoutDeadline`.
  - Adapter: `TestCredentialStreamWaitsForUserFromPromptUntilResult` (`invoke_wait_test.go`: the wait
    spans the prompt and polling, a refused prompt never waits, a stream failure still ends the wait).
  - End to end with a real plugin process (`interactive_timeout_test.go`):
    `TestDeviceSignInLongerThanTimeoutSucceeds`, `TestInteractiveHangWithoutPromptIsStillCutOffByTimeout`,
    `TestStalledSignInIsCutOffByInteractionLimit`.
  - Bridge: `TestListenPausesCallTimeoutOnlyWhileWaitingForCallback` (callback received, callback
    canceled, interaction refused).

### 3. New key `plugins.packages.<alias>.interaction_timeout`

- Problem: the sign-in limit was fixed at 15 minutes and could only be changed in Go. Some identity
  providers give users longer to approve a sign-in.
- Fix: a new optional per-package Go duration.
  - `schema.PluginPackage.InteractionTimeout` (`pkg/schema/plugins.go`); parsed by
    `config.PackageInteractionTimeout` (`pkg/plugin/config/interaction_timeout.go`).
  - Unset keeps the 15m default (`native.DefaultInteractionLimit`). A set value must be positive and at
    most 24h (`MaxInteractionTimeout`). It requires `timeout`, because without one a call has no deadline
    to extend. It applies only to interactive calls.
  - `config.Validate` reports an invalid value or a missing `timeout` with `ErrPluginInvalidConfig`, the
    package alias and key, and a hint (a separate hint when the value exceeds 24h). Disabled packages are
    not checked (`pkg/plugin/config/validate.go`).
  - The value and the package alias flow from bootstrap (`parsePackageLimits`,
    `activatePackageWithLimits` in `pkg/plugin/bootstrap/bootstrap.go`) through `extension.Activation`
    (`Alias`, `InteractionLimit`) to the auth `Definition` (`descriptors.go`, `provider.go`) and into the
    native handle (`native.WithInteractionLimit`, `native.WithPackageAlias`). The alias lets the
    sign-in-limit hint name the exact key; without one it says `<alias>`.
  - The atmos.yaml JSON schema is regenerated (`pkg/datafetcher/schema/atmos/config/1.0.json`).
- Docs: `website/docs/cli/configuration/plugins.mdx` (example, key definition, and a note that the
  timeout does not include the time the user spends signing in), `website/docs/cli/commands/plugin/usage.mdx`
  (key definition and the updated `timeout` description), and the `atmos-plugins` agent skill
  (`agent-skills/skills/atmos-plugins/SKILL.md`).
- Tests:
  - Config (`pkg/plugin/config/interaction_timeout_test.go`):
    `TestPackageInteractionTimeoutRoundTripsAndParses`, `TestPackageInteractionTimeoutAcceptsTheMaximum`,
    `TestValidateRejectsInvalidInteractionTimeout`, `TestValidateRequiresTimeoutBesideInteractionTimeout`,
    `TestValidateIgnoresInteractionTimeoutOfDisabledPackage`,
    `TestAtmosConfigSchemaDescribesPackageInteractionTimeout`.
  - Plumbing (`pkg/plugin/bootstrap/timeout_test.go`): `TestPackageInteractionTimeoutReachesTheActivation`,
    `TestAuthDefinitionCarriesTheInteractionLimit`.
  - Runtime (`pkg/plugin/runtime/native/interaction_test.go`):
    `TestConfiguredInteractionLimitAllowsSignInLongerThanDefault`,
    `TestInteractionTimeoutErrorNamesTheConfigurationKey`, `TestWithPackageAliasRecordsTheAlias`,
    `TestWithInteractionLimitIgnoresNonPositive`.

### 4. Interactive hard deadline survives a huge `timeout`

- Problem: `timeout` has no upper bound. A very large value plus the interaction limit wrapped to a
  negative duration, which skipped the deadline and left the interactive call unbounded.
- Fix: `saturatingDurationAdd` caps the sum at the largest `time.Duration`
  (`pkg/plugin/runtime/native/interaction.go`).
- Tests: `TestSaturatingDurationAdd`, `TestInteractiveDeadlineSurvivesHugeCallTimeout`
  (`interaction_overflow_test.go`).

Test-only refactors: `receiveCredentials` and `finishInvocation` callers in existing tests pass the new
arguments, the bootstrap tests use `activatePackageWithLimits`, and the device-code e2e fixture gained a
slow-device variant (`sendDeviceCredentials`, `slowDeviceFixtures`).

The `plugins.mdx` edit also replaces an en dash in "4-32 ASCII letters" with a hyphen.

## Validation

All commands ran in the PR worktree on top of `origin/main` (`2d7822f214`).

- `go build ./...`: passed.
- `go vet` on `./pkg/plugin/adapter/auth`, `./pkg/plugin/bootstrap`, `./pkg/plugin/bridge`,
  `./pkg/plugin/config`, `./pkg/plugin/extension`, `./pkg/plugin/runtime/native`, `./pkg/schema`,
  `./pkg/datafetcher`: passed.
- `go test -count=1 -cover` on the same eight packages: all passed.
- `go test -count=1 ./pkg/config/schema/...`: passed. Its drift test confirms the regenerated
  `1.0.json` matches the Go schema.
- The PR does not touch `./cmd` or `./internal/exec`, so no tests ran there.

Statement coverage, `origin/main` versus this PR (`go test -count=1 -cover`, run in a temporary detached
worktree of `origin/main` that was removed afterwards):

| Package | Before | After |
|---|---|---|
| `pkg/plugin/adapter/auth` | 95.9% | 95.9% |
| `pkg/plugin/bootstrap` | 91.0% | 91.1% |
| `pkg/plugin/bridge` | 94.1% | 94.2% |
| `pkg/plugin/config` | 92.0% | 92.2% |
| `pkg/plugin/extension` | 97.7% | 97.7% |
| `pkg/plugin/runtime/native` | 91.2% | 92.7% |
| `pkg/schema` | 91.0% | 91.0% |
| `pkg/datafetcher` | 56.0% | 56.0% |

Not run:

- The website build (`cd website && npm run build`) for the two changed `.mdx` pages.
- golangci-lint (`atmos lint --changed`); CI lint is the first lint pass.
- Manual sign-in against a real identity provider; the e2e tests use a fixture plugin process.

## Follow-ups

- **New config key and release label.** `plugins.packages.<alias>.interaction_timeout` is new
  configuration, so the PR carries the `minor` label. Its blog post is handled separately from this
  record.
- **`interaction_timeout` bounds.** It is capped at 24 hours and requires `timeout`. Letting plugins
  report the device-code expiry, so the host could size the wait itself, would need a protocol change.
  This is an open design choice, not required work.
- **Compatibility.** With `timeout` set, an interactive plugin call can now run up to `timeout` plus
  `interaction_timeout` (15m by default) in wall-clock time, instead of `timeout`. The plugin also sees
  that longer deadline in its invocation. Noninteractive calls and packages without `timeout` behave as
  before.
