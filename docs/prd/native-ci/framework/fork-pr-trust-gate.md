# Native CI Integration - Fork-PR Safety Gate for `atmos git clone`

> Related: [Overview](../overview.md) | [CI Detection](./ci-detection.md) | [Base Resolution](./base-resolution.md) | [GitHub Provider](../providers/github/provider.md)

## Problem Statement

`atmos git clone` is Atmos's native replacement for `actions/checkout`. When `ci.enabled` is
true and a CI provider is detected, the no-arg form clones the current CI repository from
environment variables instead of requiring a wrapper action
(`cmd/git/clone.go` → `runCloneNoArg` → `runCICheckout`; metadata from the GitHub provider's
`Context()` in `pkg/ci/providers/github/provider.go`):

- `CloneURL = GITHUB_SERVER_URL + "/" + GITHUB_REPOSITORY + ".git"` — always the **base** repo.
- branch = `GITHUB_REF` (overridable via `--branch`); SHA = git HEAD → `GITHUB_SHA`.

On 2026-06-18, GitHub [hardened `actions/checkout`](https://github.blog/changelog/2026-06-18-safer-pull_request_target-defaults-for-github-actions-checkout/)
(v7): it now **refuses to fetch fork PR code in `pull_request_target` and `workflow_run`** by
default. That combination — checking out untrusted fork code in a job that holds the base
repository's secrets and `GITHUB_TOKEN` — is the classic "pwn request." The refusal triggers
on explicit unsafe inputs (`ref: refs/pull/<N>/merge`, `ref: <head.sha>`,
`repository: <head.repo.full_name>`), and the opt-out is an intentionally grep-able input,
`allow-unsafe-pr-checkout`, "named to be easy to spot in code review and static analysis."

Because `atmos git clone` occupies the same role as `actions/checkout`, it inherits the same
threat surface — but today it has **no equivalent guard**.

### What is already safe

The no-arg default clones `GITHUB_REPOSITORY` (always the base repo) at `GITHUB_REF`, and
`GITHUB_REF` is event-dependent:

| Event | `GITHUB_REF` | No-arg clone checks out | Privilege | Risk |
|-------|--------------|-------------------------|-----------|------|
| `pull_request_target` | `refs/heads/<base>` | base branch code | elevated (base secrets) | safe — base code, not fork |
| `pull_request` | `refs/pull/<N>/merge` | fork-merge code | low (fork secrets withheld) | acceptable — no privileged creds |
| `push` | `refs/heads/<branch>` | branch code | elevated | safe — trusted branch |

**The no-arg default does not reproduce the pwn-request.** This PRD does not change it.

### The gap

v7 hardens the **explicit unsafe override**, and Atmos has none. In a `pull_request_target` or
`workflow_run` job — where base-repo secrets, `GITHUB_TOKEN`, and cloud credentials are present
— a user can explicitly point `atmos git clone` at untrusted fork content and Atmos complies:

- `atmos git clone --branch refs/pull/<N>/merge` — overrides `ciCtx.Ref` in `runCICheckout`,
  pulling the fork-merge ref of the base repo.
- `atmos git clone <fork-uri>` — the ad-hoc URI form (`runCloneURI`) clones any URL verbatim.

Atmos then *executes* whatever it cloned: Go template rendering, Gomplate datasources, YAML
functions (`!exec`, `!terraform.output`, `!store`), custom commands, hooks, and terraform
itself. Fork-authored stack config running with base-repo credentials is the Atmos pwn request.

Atmos currently has **zero fork/trust awareness**:

- `pkg/ci/providers/github/provider.go:123` treats `pull_request` and `pull_request_target`
  identically; `parsePRInfo()` reads `GITHUB_BASE_REF`/`GITHUB_HEAD_REF` but never
  `head.repo.fork` or `head.repo.full_name` vs `base.repo.full_name`.
- `runCICheckout` / `runCloneURI` clone whatever ref/URI they are handed.
- `workflow_run` is not modeled by the provider at all.
- The only existing security requirement ([overview.md](../overview.md) NFR-3) covers "don't
  log secrets / scope the token" — nothing about fork-clone trust.

## FR-30: Untrusted-Fork Detection

**Requirement**: The CI provider exposes a trust signal indicating whether a requested clone
under the current event would fetch **untrusted fork content**. The signal rides on the
existing CI `Context` (which already populates `EventName`, `Ref`, `Repository`, and `PRInfo`).

**Dangerous combination** = elevated event **AND** fork-targeting clone:

- **Elevated event**: `GITHUB_EVENT_NAME ∈ {pull_request_target, workflow_run}`.
- **Fork-targeting clone** — the *requested clone* points at fork content (either one of):
  1. A `--branch`/ref override that is a PR merge or head ref (e.g. `refs/pull/<N>/merge`,
     `refs/pull/<N>/head`).
  2. An ad-hoc clone URI whose host **or** `owner/repo` differs from the base
     (`GITHUB_SERVER_URL` + `GITHUB_REPOSITORY`).

The signal is keyed off the **requested clone target**, not merely the event payload. A
forked PR under `pull_request_target` is *not* by itself fork-targeting: the safe no-arg
checkout (base repository at its base ref) stays trusted. Payload fields such as
`event.pull_request.head.repo.fork` may *corroborate* a cross-repo URI but never gate the
default base checkout on their own.

**Behavior**:
- Detection is provider-supplied so non-GitHub providers can contribute their own signals.
- Absence of a payload must not weaken the gate — both signals are derivable from the
  command invocation and env alone.

**Validation**:
- `pull_request_target` + `--branch refs/pull/5/merge` → untrusted.
- `pull_request_target` + no-arg (base repo, base ref) → trusted.
- `pull_request` (any target) → not gated (low-privilege event).
- `push` → not gated.

## FR-31: Default Hard-Refuse

**Requirement**: When `atmos git clone` detects the dangerous combination (FR-30), it
**errors and exits before cloning** unless explicitly opted in (FR-32).

**Behavior**:
- Uses the error builder with an actionable hint (per repo error conventions), e.g.:
  "Refusing to clone fork pull-request content in a `pull_request_target`/`workflow_run`
  workflow, which runs with this repository's secrets." Hints point to the opt-in and to the
  recommended `pull_request` workflow (FR-33).
- Exit code is non-zero (deliberate, fail-closed).
- **Unaffected**: the no-arg default, same-repo clones, and non-elevated events (`pull_request`,
  `push`, `merge_group`, local runs) clone exactly as today.

**Validation**:
- Dangerous combination without opt-in → non-zero exit, nothing cloned.
- Same scenario with opt-in → clone proceeds.
- Non-elevated event with a fork ref → clone proceeds (not gated).

## FR-32: Explicit, Grep-able Opt-In

**Requirement**: Opting out of the gate is explicit and intentionally easy to spot in review
and static analysis (the v7 design intent behind `allow-unsafe-pr-checkout`).

**Surface** (standard Atmos precedence: flag → env → config → default):
- Flag: `--allow-unsafe-fork`
- Env: `ATMOS_ALLOW_UNSAFE_FORK_EXECUTION`
- Config: `ci.allow_unsafe_fork_execution: true` in `atmos.yaml`
- Default: `false`

**Behavior**:
- When set, the gate logs a prominent warning (so the bypass is visible in CI logs) and the
  clone proceeds.
- The name deliberately contains "unsafe" so a grep/policy scan over workflows and config
  flags every bypass.

**Validation**:
- Each surface independently enables the bypass; precedence matches the rest of Atmos.

## FR-33: Workflow Guidance (Documentation)

**Requirement**: User docs recommend the safe pattern.

**Behavior**:
- Prefer `pull_request` (not `pull_request_target`) for workflows that clone+plan fork
  contributions; `pull_request` withholds fork secrets, so untrusted code never meets
  privileged credentials.
- Reserve `pull_request_target` / `workflow_run` for trusted, secret-free steps (e.g. labeling,
  comment formatting), or gate them behind `--allow-unsafe-fork` only with a documented reason.

## Behavior Matrix

| Event | Clone target | Opt-in | Result |
|-------|--------------|--------|--------|
| `pull_request_target` | no-arg (base repo, base ref) | — | clone (trusted) |
| `pull_request_target` | `--branch refs/pull/<N>/merge` | no | **refuse** |
| `pull_request_target` | `--branch refs/pull/<N>/merge` | yes | clone (warn) |
| `pull_request_target` | fork URI (host or `owner/repo` ≠ base) | no | **refuse** |
| `workflow_run` | fork ref / fork URI | no | **refuse** |
| `workflow_run` | fork ref / fork URI | yes | clone (warn) |
| `pull_request` | any | — | clone (low-privilege event) |
| `push` / `merge_group` / local | any | — | clone |

## Out of Scope / Future

- JSON schema modeling of the `ci:` block in `pkg/datafetcher/schema` — that block is not
  schematized today (not even `ci.enabled`/`ci.cache`), so the new
  `ci.allow_unsafe_fork_execution` key is added to the Go config struct only, deferred to a
  broader `ci:`-schema effort.
- Non-GitHub providers (GitLab MR pipelines, etc.). The gate lives in the provider-agnostic
  clone path (`cmd/git`) and consumes a provider-supplied trust signal; each provider adds its
  own fork-detection over time (see the [provider registry](../providers/README.md)).
- Hardening of execution paths *other* than clone (e.g. a contributor with write access to the
  base repo) — out of scope; this gate addresses the fork/elevated-event surface only.

## References

- actions/checkout v7 changelog: https://github.blog/changelog/2026-06-18-safer-pull_request_target-defaults-for-github-actions-checkout/
- `cmd/git/clone.go` — `runCloneNoArg`, `runCICheckout`, `runCloneURI`
- `pkg/ci/providers/github/provider.go` — `Context()`, `parsePRInfo()`
- [overview.md](../overview.md) — NFR-3 Security
