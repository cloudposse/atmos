# Fix: `describe affected --repo-path` fails when `ci.enabled: true`

**Date:** 2026-04-21

**Issue:**

- Atmos 1.216.0 (cloudposse/atmos#2241) introduced CI base
  auto-detection in `atmos describe affected`. When
  `ci.enabled: true` is set in `atmos.yaml` (or `--ci` is passed),
  the command auto-injects a `--base` value derived from the CI
  provider's environment (`GITHUB_BASE_REF`, `GITHUB_SHA`, the
  `pull_request` event payload, etc.) when the user did not supply
  `--base` / `--ref` / `--sha` explicitly.
- `atmos describe affected` also supports `--repo-path`, which points
  at an already-cloned sibling repo to diff against. `--repo-path`
  is **mutually exclusive** with `--base` / `--ref` / `--sha` /
  `--ssh-key` / `--ssh-key-password` and the CLI rejects the
  combination with `ErrRepoPathConflict`:

  ```text
  Error: if the '--repo-path' flag is specified, the '--base',
  '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags
  can't be used
  ```

- The auto-detection in 1.216.0 runs unconditionally whenever
  `ci.enabled: true`, so a user who opts into Atmos Pro's CI
  features (GitHub Check Runs, `$GITHUB_OUTPUT` exports,
  `$GITHUB_STEP_SUMMARY` rendering) **also** gets auto-injected
  `--ref` / `--sha`, even on invocations that supply `--repo-path`.
  The result: `ErrRepoPathConflict` fires and every
  `describe affected --repo-path …` call inside a GitHub Actions
  PR check aborts.
- The failure surfaced in production via the
  `cloudposse/github-action-atmos-affected-trigger-spacelift`
  action, which **always** passes `--repo-path`
  (`trigger-spacelift@v2.2.2`, `affected-stacks@v6.13.0`). Users who
  turned on `ci.enabled: true` in `atmos.yaml` for the Atmos Pro
  workflows broke every spacelift-trigger PR check on the same
  repo.
- There is today no runtime override: `ci.enabled` is only read
  from `atmos.yaml`, and the only CLI escape hatch (`--ci`) only
  forces CI mode on — it cannot turn it off. Users cannot scope
  `ci.enabled: true` to specific workflows without editing
  `atmos.yaml` inline (the yq-patch workaround reported in a
  downstream repository).

**User-visible failure:**

```text
[INFO] Auto-detected CI base provider=github-actions event=pull_request
       base=refs/remotes/origin/main source=GITHUB_BASE_REF
Error: if the '--repo-path' flag is specified, the '--base',
'--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't
be used
```

The `INFO` line is the smoking gun — the CI provider injected the
base and then the validator rejected the result.

## Status

**Implemented.** All code changes and unit tests landed in branch
`aknysh/fix-atmos-ci-2`; user-facing documentation updated.

## Goals

1. `atmos describe affected --repo-path=<path>` must succeed on PR
   CI even when `ci.enabled: true` is set in `atmos.yaml`, without
   requiring the user to patch `atmos.yaml` per workflow.
2. Preserve the auto-detection behavior for every other invocation
   shape (`describe affected` without `--repo-path`, with or
   without `--base` / `--ref` / `--sha`). The 1.216.0 feature
   stays, it just stops firing when it can't possibly be used.
3. Let users disable CI features for a single job / step without
   editing `atmos.yaml`, by extending the existing `ATMOS_CI` /
   `CI` env vars (already bound to `--ci` on
   `terraform plan`/`apply`/`deploy`) so they also gate the
   describe-affected auto-base-detect path.
4. Add a `--ci` flag to `describe affected` so it parallels
   `terraform plan` / `apply` / `deploy`. `--ci=false` explicitly
   disables CI-gated behavior (auto base detection) for that
   invocation; `--ci=true` forces it on even when `ci.enabled`
   is not set in config. Flag precedence: CLI flag > env var >
   `ci.enabled` in config > default.
5. Produce a clear error message when the user explicitly passes
   both `--repo-path` and `--base` / `--ref` / `--sha`. The
   existing `ErrRepoPathConflict` stays as-is for explicit
   conflicts; it just stops firing for auto-detected values.

## Non-goals

- **Rewriting `ci.enabled` semantics.** `ci.enabled: true` in
  `atmos.yaml` remains the authority for CI hooks (checks,
  summaries, outputs, plan-file stores). This fix only narrows
  the `describe affected` base-auto-detect subfeature so it does
  not inject conflicting flags.
- **Making `--repo-path` compatible with `--base` / `--ref` /
  `--sha`.** These remain mutually exclusive — the user's explicit
  flags still produce `ErrRepoPathConflict`. Only the
  auto-detection is suppressed.
- **Changing the upstream GitHub Action.** Not needed. `--repo-path`
  alone already suppresses auto-detect after Fix 1, so the existing
  `cloudposse/github-action-atmos-affected-trigger-spacelift`
  (and `-stacks`) actions work as-is once callers upgrade Atmos.
- **Making every CI consumer honor `ATMOS_CI=false`.** This fix
  scopes the flag/env override to `describe affected` so the blast
  radius is narrow. Extending the same override semantics to
  `pkg/hooks`, `pkg/list/list_instances.go`, and
  `internal/exec/ci_exit_codes.go` is tracked as a follow-up so
  each call site can be reviewed individually.

## Implementation

### Fix 1 — Skip CI base auto-detect when `--repo-path` is set

File: `internal/exec/describe_affected.go`, function
`SetDescribeAffectedFlagValueInCliArgs`.

Current code (lines 236–239):

```go
// Auto-detect base from CI environment when ci.enabled is true and
// no explicit base provided.
if describe.Ref == "" && describe.SHA == "" &&
    describe.CLIConfig != nil && describe.CLIConfig.CI.Enabled {
    resolveBaseFromCI(describe)
}
```

New code:

```go
// Auto-detect CI metadata when CI is enabled and no explicit base
// provided.
//
// Two cases:
//   - Normal (no --repo-path): populate Ref/SHA + HeadSHAOverride/
//     CIEventType from the CI provider.
//   - --repo-path: skip base detection (--repo-path is mutually
//     exclusive with --base/--ref/--sha per ErrRepoPathConflict —
//     auto-injecting from CI env would break every call that uses
//     --repo-path, e.g. the cloudposse/github-action-atmos-
//     affected-trigger-spacelift action). When --upload is also
//     set, still populate HeadSHAOverride + CIEventType so Atmos
//     Pro can match the uploaded affected list to the PR webhook
//     SHA — the current checkout is typically a merge commit,
//     whose SHA does not match what the webhook indexed.
if describe.Ref == "" && describe.SHA == "" &&
    isCIEnabledForDescribeAffected(describe) {
    switch {
    case describe.RepoPath == "":
        resolveBaseFromCI(describe)
    case describe.Upload:
        resolveCIUploadMetadataFromCI(describe)
    }
}
```

Impact on `ErrRepoPathConflict`: the validator on line 169 is
unchanged. Users who explicitly pass `--repo-path` *and* `--base`
still get the same error. Only auto-detected `Ref`/`SHA` are
suppressed.

### Fix 1b — Partial CI resolver for `--repo-path + --upload`

`--repo-path + --upload` on a CI PR event previously errored out
with `ErrRepoPathConflict`. With Fix 1 alone, the combination
succeeds but the upload uses the current checkout's local HEAD
as the correlation SHA — which on GitHub Actions default
`actions/checkout@v4` is a merge-commit SHA, not the PR head
SHA indexed by the webhook. Atmos Pro would then fail to
correlate the upload with the PR.

A new sibling of `resolveBaseFromCI` populates **only** the
upload metadata (HeadSHAOverride, CIEventType), leaving
`Ref`/`SHA` untouched so `ErrRepoPathConflict` does not fire:

```go
// resolveCIUploadMetadataFromCI populates the upload correlation
// metadata (HeadSHAOverride, CIEventType) from the CI provider
// WITHOUT populating Ref or SHA.
//
// Used in the --repo-path + --upload + CI code path. Base
// auto-detection is skipped there to avoid ErrRepoPathConflict,
// but Atmos Pro still needs the PR head SHA and event type to
// correlate the uploaded affected list with the PR webhook —
// the current checkout is typically a GitHub pull_request merge
// commit, whose SHA does not match what the webhook indexed.
func resolveCIUploadMetadataFromCI(describe *DescribeAffectedCmdArgs) {
    defer perf.Track(nil, "exec.resolveCIUploadMetadataFromCI")()

    p := ci.Detect()
    if p == nil {
        return
    }
    resolution, err := p.ResolveBase()
    if err != nil || resolution == nil {
        return
    }

    describe.HeadSHAOverride = resolution.HeadSHA
    describe.CIEventType = resolution.EventType
}
```

The helper intentionally reuses `p.ResolveBase()` — provider
detection and event-payload parsing are identical to the base
case; only the fields copied to `describe` differ.

### Fix 2 — `--ci` flag on `describe affected` (env-bound to `ATMOS_CI`, `CI`)

File: `cmd/describe_affected.go` — register the flag through the
unified flag parser (same pattern used by
`cmd/terraform/plan.go:119`):

```go
describeAffectedCIParser = flags.NewStandardParser(
    flags.WithBoolFlag("ci", "", false,
        "Enable CI mode for `describe affected`. Overrides "+
        "ci.enabled in atmos.yaml for this invocation."),
    flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
)
describeAffectedCIParser.RegisterFlags(describeAffectedCmd)
_ = describeAffectedCIParser.BindToViper(viper.GetViper())
```

The env-var bindings deliberately reuse the existing `ATMOS_CI` /
`CI` pair — no new env var is introduced. `ATMOS_CI` already ships
(PR #2079, March 2026) bound to the `--ci` flag on
`terraform plan`/`apply`/`deploy`. Reusing it gives users a single,
consistent knob across every CI-aware command.

File: `internal/exec/describe_affected.go` — introduce a helper
`isCIEnabledForDescribeAffected` that applies the precedence
CLI flag > `ATMOS_CI` env var > `CI` env var >
`ci.enabled` in config > `false`:

```go
// isCIEnabledForDescribeAffected returns whether CI
// auto-base-detection should run. Precedence (highest to lowest):
//   1. --ci CLI flag on this invocation (pflag.Changed).
//   2. ATMOS_CI env var (os.LookupEnv).
//   3. CI env var (os.LookupEnv).
//   4. ci.enabled in atmos.yaml.
//   5. false (default).
func isCIEnabledForDescribeAffected(flags *pflag.FlagSet, describe *DescribeAffectedCmdArgs) bool {
    if flags != nil {
        if f := flags.Lookup("ci"); f != nil && f.Changed {
            if val, err := flags.GetBool("ci"); err == nil {
                return val
            }
        }
    }
    for _, name := range []string{"ATMOS_CI", "CI"} {
        if val, ok := os.LookupEnv(name); ok && val != "" {
            if parsed, err := strconv.ParseBool(val); err == nil {
                return parsed
            }
        }
    }
    if describe.CLIConfig == nil {
        return false
    }
    return describe.CLIConfig.CI.Enabled
}
```

**Why not `viper.IsSet("ci")`?** `StandardParser.BindFlagsToViper`
(see `pkg/flags/standard.go`) calls `v.SetDefault("ci", false)` as
part of the registration flow. After `SetDefault`, `viper.IsSet("ci")`
returns `true` on every invocation — regardless of whether the user
passed `--ci` or set an env var. An earlier draft of this helper
trusted `viper.IsSet` and silently masked `ci.enabled: true` from
`atmos.yaml` with the pflag default (`false`). The regression was
caught by the `TestIsCIEnabledForDescribeAffected_RealBinding` test,
which stands up the exact production binding chain
(`SetDefault` + `BindEnv` + `BindPFlag`). Checking `pflag.Changed`
plus `os.LookupEnv` directly is the only reliable way to distinguish
explicit user overrides from the parser's default.

### Error-message hint

After Fix 1, `ErrRepoPathConflict` only fires when a user
**explicitly** passes both `--repo-path` and one of
`--base` / `--ref` / `--sha` / `--ssh-key` / `--ssh-key-password`
— auto-detected values no longer trigger the validator. The
sentinel stays as-is:

```go
var ErrRepoPathConflict = errors.New(
    "if the '--repo-path' flag is specified, the '--base', " +
    "'--ref', '--sha', '--ssh-key' and '--ssh-key-password' " +
    "flags can't be used")
```

…and we wrap it in the builder pattern per `errors/errors.go`
with two hints that explain the mutually exclusive flag groups
and guide the user toward the right flag for their intent:

```go
return errUtils.Build(ErrRepoPathConflict).
    WithHint("Pass only one of: --repo-path OR (--base | " +
        "--ref | --sha | --ssh-key | --ssh-key-password). " +
        "--repo-path points at an already-cloned sibling " +
        "repository to diff against; the others clone or " +
        "check out a target ref.").
    WithHint("To compare against a specific ref or SHA, use " +
        "--base without --repo-path. To compare against an " +
        "already-cloned repo, use --repo-path without --base " +
        "/ --ref / --sha.").
    Err()
```

The hints deliberately do NOT suggest pairing `--ci=false` /
`ATMOS_CI=false` with `--repo-path` — Fix 1 already suppresses
auto-detect when `--repo-path` is set, so that pairing is
unnecessary. The `--ci` / `ATMOS_CI` overrides (Fix 2) exist for
the independent case of toggling CI-gated behavior on
invocations that do not use `--repo-path`.

## Testing

### Unit tests — `internal/exec/describe_affected_test.go`

Add to `TestSetDescribeAffectedFlagValueInCliArgs_BaseResolution`:

1. `repo-path skips CI auto-detect even when ci.enabled=true`
   — set `GITHUB_ACTIONS=true`, `GITHUB_BASE_REF=main`, the PR
   event payload on disk, `ci.enabled=true` in config, and
   `--repo-path=/tmp/repo`. Assert `describe.Ref == ""`,
   `describe.SHA == ""`, and `describe.RepoPath ==
   "/tmp/repo"`. The absence of auto-injected values is the
   contract.
2. `ATMOS_CI=false disables auto-detect even when
   ci.enabled=true` — same GitHub Actions env, `ci.enabled=true`
   in config, `ATMOS_CI=false`. Assert `describe.Ref ==
   ""` and `describe.SHA == ""`.
3. `--ci=false flag disables auto-detect even when
   ci.enabled=true` — same GitHub Actions env, `ci.enabled=true`
   in config, flag `--ci=false`. Assert `describe.Ref == ""`
   and `describe.SHA == ""`.
4. `--ci=true enables auto-detect even when ci.enabled=false`
   — same GitHub Actions env, `ci.enabled=false` in config,
   flag `--ci=true`. Assert `describe.Ref ==
   "refs/remotes/origin/main"`.
5. `explicit --repo-path + --base still errors` — no CI env,
   `--repo-path=/tmp/repo` and `--base=main`. Assert
   `ParseDescribeAffectedCliArgs` returns `ErrRepoPathConflict`.
   This is the negative-path counterpart to (1) per
   `CLAUDE.md` § "Include negative-path tests for recovery
   logic".
6. `precedence: CLI flag > env var > config` — table-driven
   test. For each combination of `(--ci flag set, ATMOS_CI
   set, ci.enabled in config)`, assert the resolved value
   matches the documented precedence.

### Integration / manual verification

Reproduce the user-reported failure locally:

```bash
# Fresh checkout with ci.enabled: true in atmos.yaml
cat >> atmos.yaml <<'YAML'
ci:
  enabled: true
YAML

# Simulate GitHub Actions PR event.
export GITHUB_ACTIONS=true
export GITHUB_EVENT_NAME=pull_request
export GITHUB_BASE_REF=main
export GITHUB_EVENT_PATH=/tmp/event.json
cat > /tmp/event.json <<'JSON'
{
  "action": "synchronize",
  "pull_request": {
    "head": {"sha": "0000000000000000000000000000000000000001"},
    "base": {"ref": "main"}
  }
}
JSON

# Before fix: fails with ErrRepoPathConflict.
# After fix: succeeds; no auto-detect, uses --repo-path only.
atmos describe affected --repo-path=/tmp/other-clone
```

Then verify the new overrides. Note: `--repo-path` alone already
suppresses auto-detection (Fix 1), so `--ci=false` is **not** needed
for the `--repo-path` case. The `--ci` / `ATMOS_CI` overrides are
for the independent case of toggling CI-gated behavior on
invocations that do *not* use `--repo-path`:

```bash
# Opt one non-repo-path invocation out of CI auto-detect while
# leaving ci.enabled: true in atmos.yaml for other features.
ATMOS_CI=false atmos describe affected --base main
atmos describe affected --base main --ci=false

# Explicitly force CI mode even with ci.enabled: false in config —
# e.g., reproducing CI behavior locally.
unset ATMOS_CI
# Edit atmos.yaml to set ci.enabled: false, then:
atmos describe affected --ci=true  # should auto-detect base
```

### CI regression coverage

The existing `TestSetDescribeAffectedFlagValueInCliArgs_BaseResolution/CI_auto-detect_when_enabled_and_no_explicit_base`
test continues to exercise the auto-detect path. No change needed
there — we are only adding a new guard condition, not removing
behavior.

## Progress checklist

- [x] Fix doc (this file).
- [x] Fix 1: skip `resolveBaseFromCI` when `describe.RepoPath != ""`
  (`internal/exec/describe_affected.go`,
  `SetDescribeAffectedFlagValueInCliArgs`).
- [x] Fix 1b: new `resolveCIUploadMetadataFromCI` helper that
  populates only `HeadSHAOverride` + `CIEventType` on the
  `--repo-path + --upload + CI` path, so Atmos Pro retains PR
  webhook correlation even though `--base` auto-detection is
  skipped (`internal/exec/describe_affected.go`).
- [x] Fix 2a: `--ci` flag on `describe affected`
  (`cmd/describe_affected.go`, `describeAffectedCIParser`).
- [x] Fix 2b: env binding via
  `flags.WithEnvVars("ci", "ATMOS_CI", "CI")` (reuses the existing
  env-var pair — no new env var added).
- [x] Fix 2c: `isCIEnabledForDescribeAffected` helper with
  documented precedence (`--ci` via `pflag.Changed` > `ATMOS_CI` /
  `CI` via `os.LookupEnv` > `ci.enabled` in config > `false`).
  Intentionally does NOT use `viper.IsSet("ci")` — after
  `StandardParser.BindFlagsToViper` calls `SetDefault`, `IsSet`
  returns `true` on every invocation and would mask the config
  fallback.
- [x] Fix 3: `ErrRepoPathConflict` error wrapped with
  `errUtils.Build(...).WithHint(...).Err()` explaining the
  mutually exclusive flag groups (no longer points at `--ci=false` /
  `ATMOS_CI=false`, which Fix 1 makes unnecessary for the
  `--repo-path` case).
- [x] Unit tests:
  `TestSetDescribeAffectedFlagValueInCliArgs_RepoPathSkipsCIAutoDetect`
  (includes inline negative-path sanity assertion),
  `TestSetDescribeAffectedFlagValueInCliArgs_RepoPathUploadPopulatesMetadata`
  (asserts `HeadSHAOverride` + `CIEventType` populated from event
  payload while `Ref`/`SHA` stay empty; plus a negative-path
  assertion that dropping `--upload` keeps metadata empty),
  `TestSetDescribeAffectedFlagValueInCliArgs_CIFlagOverrides`
  (4 sub-tests),
  `TestIsCIEnabledForDescribeAffected_Precedence`
  (5 sub-tests).
- [x] Documentation:
  `website/docs/cli/commands/describe/describe-affected.mdx` —
  new `--ci` flag entry, `--repo-path` interaction note, and
  "Overriding CI auto-detection per invocation" section with
  precedence table.
- [x] Documentation: `website/docs/cli/configuration/ci/index.mdx`
  — `ATMOS_CI` row added to the env-var table.
- [x] Documentation:
  `website/docs/cli/environment-variables.mdx` — new
  "CI Integration" section covering `ATMOS_CI` (extended to gate
  describe-affected CI auto-detection) and `CI`.
- [ ] Blog post: `website/blog/2026-04-21-describe-affected-repo-path-ci-fix.mdx`
  (labeled `bugfix`).
- [ ] Roadmap update: add milestone under the `native-ci`
  initiative, `status: 'shipped'`, link PR + blog slug.
- [ ] Build website: `cd website && npm run build`.
- [ ] `make lint && make testacc` clean (full suite pending).

## Follow-ups

- **No upstream GitHub Action change required.** `--repo-path`
  alone suppresses CI base auto-detection after Fix 1, so the
  existing `cloudposse/github-action-atmos-affected-stacks` and
  `-trigger-spacelift` actions work as-is once callers upgrade
  Atmos. Any downstream yq-patch workaround in a consumer repo
  can be reverted on upgrade.
- **Make every CI consumer honor `ATMOS_CI=false`.** Extend the
  same env override to `pkg/hooks/hooks.go:136`,
  `internal/exec/ci_exit_codes.go:23`,
  `pkg/list/list_instances.go:596`, and `cmd/ci/status.go:61`.
  Each site reads `atmosConfig.CI.Enabled` directly today; a
  shared helper (same semantics as `isCIEnabledForDescribeAffected`) would
  centralize the precedence rule. Deferred to a separate PR so
  each call site can be reviewed individually — the semantics of
  "disable CI features mid-workflow" differ per consumer.
- **Deprecate `ci.enabled`-only gating for auto base detection.**
  Longer term, consider decoupling base auto-detect from
  `ci.enabled`. The two serve different audiences: `ci.enabled`
  is for CI outputs (checks, summaries), while base auto-detect
  is a DX improvement that could run whenever a known CI
  provider is detected. Gated behind a separate knob (e.g.
  `ci.describe_affected.auto_detect_base`) so users can keep the
  Atmos Pro features without the flag-injection side effect.
  Requires a minor-version release and migration notes —
  deferred.
- **Structured error for CLI flag conflicts.** `ErrRepoPathConflict`
  today is a single sentinel covering five flag names. Splitting
  into `ErrRepoPathConflictWithBase`,
  `ErrRepoPathConflictWithRef`, etc. would make error handling
  and documentation cleaner. Out of scope here.

---

## Related

- `internal/exec/describe_affected.go` — `ErrRepoPathConflict`,
  `SetDescribeAffectedFlagValueInCliArgs`, `resolveBaseFromCI`.
- `cmd/describe_affected.go` — `describe affected` flag
  declarations.
- `cmd/terraform/plan.go:119` — reference pattern for the
  `flags.WithEnvVars("ci", "ATMOS_CI", "CI")` binding.
- `docs/prd/native-ci/framework/ci-detection.md` — `ci.enabled`
  authority model; explains why `--ci` / `ATMOS_CI` / `CI` today
  can only *enable* CI mode, never disable it. This fix
  introduces the first runtime disable path (`--ci=false`,
  `ATMOS_CI=false`).
- `docs/prd/native-ci/framework/base-resolution.md` — CI base
  auto-detection spec introduced in cloudposse/atmos#2241.
- `errors/errors.go` — error-builder pattern
  (`Build(err).WithHint(...).Err()`).
- `pkg/flags/` — `WithEnvVars` flag-to-env-var binding
  (`pkg/flags/global_builder.go`).
- User-reported failure in a downstream consumer repo where a
  workflow had to apply a `yq '.ci.enabled = false'` patch inline
  to unblock the spacelift-trigger PR check — the workaround that
  motivated this fix.
- cloudposse/atmos#2241 — the PR that introduced CI base
  auto-detection.
