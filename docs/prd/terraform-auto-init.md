# Terraform Smart Auto-Init

**Version:** 1.0
**Last Updated:** 2026-09-11

---

## Executive Summary

Atmos has always run `terraform init -reconfigure` before every Terraform subcommand — `plan`, `apply`,
`shell`, `destroy`, and even the implicit init behind `!terraform.output` and `atmos.Component` — and before
`deploy` when `deploy_run_init` is enabled. This guarantees a working directory is always initialized, but it
also means running `atmos terraform apply demo -s dev` followed seconds later by
`atmos terraform output demo -s dev` re-initializes the same component twice: two rounds of "Initializing
provider plugins... Initializing the backend..." for a component whose providers, modules, and backend have
not changed at all.

This PRD introduces a fingerprint-based skip rule under `components.terraform.init` (`mode`, `reconfigure`,
`upgrade`) so Atmos only re-runs `terraform init` when something that actually affects it has changed, adds
`-reconfigure`/`-upgrade` only when the situation calls for it, and falls back to a safe, automatic retry when
a skipped init turns out to have been wrong. The defaults change behavior (init is no longer unconditional,
and `-reconfigure` is no longer added to every init), so both are also configurable per the previous behavior
for users who want to keep it.

---

## Problem Statement

### Current Behavior

Every Atmos Terraform command — single-component or multi-component, plus the resolution of
[`!terraform.output`](/functions/yaml/terraform.output) and [`atmos.Component`](/functions/template/atmos.Component)
— runs `terraform init` first, unconditionally, and (by default) adds `-reconfigure`. There is no way to tell
Atmos "this component's providers, modules, and backend haven't changed since the last init — skip it."

### User Impact

A common local development loop:

```shell
atmos terraform apply demo -s dev
# ... Initializing provider plugins...
# ... Initializing the backend...
# ... apply completes ...

atmos terraform output demo -s dev
# ... Initializing provider plugins...   <- nothing changed, but init runs again
# ... Initializing the backend...
```

Nothing about the component changed between the two commands, yet the second one pays the full init cost
again — provider plugin downloads (or at minimum a filesystem scan and network round-trip through the
registry), backend re-initialization, and module re-resolution. On a slow connection, or with a component that
has many providers, this can add tens of seconds to every single command, turning a fast edit-plan-apply loop
into a slow one.

This is the situation [issue #620](https://github.com/cloudposse/atmos/issues/620), "Intelligently don't init
on every apply/shell," asked Atmos to fix — requesting Terragrunt-style Auto-Init, where init only re-runs
when it needs to. [Issue #1263](https://github.com/cloudposse/atmos/issues/1263) asked, separately, for a way
to have Atmos run `init -upgrade` when appropriate, rather than requiring users to pass `-upgrade` by hand
after every provider version bump.

### Why Existing Workarounds Fall Short

`--skip-init` exists today, but it is all-or-nothing and per-invocation: users must remember to pass it, and
if the working directory actually does need re-initializing (a provider constraint changed, a module was
added), `--skip-init` produces a hard failure instead of the correct outcome. There is no middle ground
between "always re-init" and "the user manually judges when it's safe to skip."

---

## Goals

- **Skip init when nothing that affects it changed.** Reduce the common case (`apply` then `output`, or
  repeated `plan`/`apply` cycles during development) to zero redundant init calls.
- **Add `-reconfigure` and `-upgrade` only when warranted**, not on every init, while keeping an escape hatch
  (`always`) for users who want the previous unconditional behavior.
- **Stay safe under a wrong skip.** If Atmos skips an init that turned out to be necessary, recognize the
  resulting Terraform/OpenTofu failure and recover automatically, without ever risking state.
- **Apply uniformly** across explicit CLI commands and the implicit init that
  `!terraform.output`/`atmos.Component` evaluation performs.
- **Preserve exact backward compatibility** for anyone who sets `init.mode: always` and
  `init.reconfigure: always`, and — for `init.mode`/`init.upgrade` specifically — automatically for
  anyone whose project is pinned to an [edition](/cli/configuration/edition) from before
  2026-09-12, with no config changes at all (see [Editions](#editions)). `init.reconfigure` is the
  exception: leaving the legacy `init_run_reconfigure: true` default does **not** preserve
  unconditional `-reconfigure`, and pinning an edition does **not** restore it either — it now maps
  to `init.reconfigure: auto` regardless of pin — see
  [Migration](#migration-from-init_run_reconfigure).

## Non-Goals

- **No state migration.** Auto-recovery never runs `-migrate-state`; a backend change that requires state
  migration always surfaces as an explicit, user-driven `terraform init -migrate-state` or
  [`atmos terraform migrate`](/cli/commands/terraform/migrate) step.
- **No user-extensible diagnostic list.** The set of Terraform/OpenTofu diagnostics that trigger auto-recovery
  is fixed and shipped with Atmos (see [Auto-Recovery Contract](#auto-recovery-contract)); it is not a
  configuration surface. Extending it is a future enhancement, not part of this PRD.
- **No fingerprinting of nested module contents.** See [Known Gaps](#known-gaps).
- **No change to `deploy_run_init` or `--skip-init` semantics.** Both continue to work exactly as before;
  `init.mode` governs what happens on the runs `--skip-init` doesn't suppress.

---

## Proposed Solution

### Configuration

```yaml
components:
  terraform:
    init:
      mode: auto         # auto | always | never
      reconfigure: auto  # auto | always | never
      upgrade: auto       # auto | always | never
      pass_vars: false    # existing setting, unchanged
```

All three settings default to `auto`. `init.mode` and `init.upgrade` reach that default through an
[edition](/cli/configuration/edition) journal entry rather than a plain struct literal, so a
project pinned to an edition from before 2026-09-12 sees `always` and `never` respectively instead
— see [Editions](#editions) below for why, and why `init.reconfigure` can't use the same
mechanism.

<dl>
  <dt><code>init.mode</code></dt>
  <dd>
    <code>auto</code> skips <code>terraform init</code> when nothing that affects init has changed since the
    last successful init for this component/workspace. <code>always</code> restores the previous
    unconditional behavior (the behavior a pre-2026-09-12 edition pin restores). <code>never</code> never
    runs init implicitly — the same effect as passing <code>--skip-init</code> on every invocation.<br/>
    <strong>Environment variable:</strong> <code>ATMOS_COMPONENTS_TERRAFORM_INIT_MODE</code><br/>
    <strong>Command-line flag:</strong> <code>--init-mode</code>
  </dd>
  <dt><code>init.reconfigure</code></dt>
  <dd>
    <code>auto</code> adds <code>-reconfigure</code> only when the backend configuration changed since the
    last init, on the first init, after a JIT working directory is re-provisioned, or for
    <code>atmos terraform workspace</code>. <code>always</code> adds it to every init (the previous default).
    <code>never</code> never adds it.<br/>
    <strong>Environment variable:</strong> <code>ATMOS_COMPONENTS_TERRAFORM_INIT_RECONFIGURE</code><br/>
    <strong>Command-line flag:</strong> <code>--init-reconfigure</code>
  </dd>
  <dt><code>init.upgrade</code></dt>
  <dd>
    <code>auto</code> adds <code>-upgrade</code> only when Terraform/OpenTofu reports that an upgrade is
    required (e.g. a provider version constraint was raised beyond the locked version). <code>always</code>
    adds it to every init. <code>never</code> never adds it (the behavior a pre-2026-09-12 edition pin
    restores).<br/>
    <strong>Environment variable:</strong> <code>ATMOS_COMPONENTS_TERRAFORM_INIT_UPGRADE</code><br/>
    <strong>Command-line flag:</strong> <code>--init-upgrade</code>
  </dd>
</dl>

### Precedence

Standard Atmos precedence applies to all three settings: **CLI flag > environment variable > `atmos.yaml` >
default (`auto`)**. `--skip-init` is unchanged and is equivalent to `--init-mode=never` for that one
invocation — it does not change the persisted `atmos.yaml` setting. `deploy_run_init` is unchanged and
continues to control only whether `deploy` runs init *at all*; when it does, `init.mode` governs whether that
init is actually executed or skipped.

### Migration from `init_run_reconfigure`

The existing boolean `init_run_reconfigure` (env `ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE`, flag
`--init-run-reconfigure`) keeps working and is now documented as deprecated in favor of `init.reconfigure`.

- If `init.reconfigure` is set explicitly (at any precedence level), it wins outright.
- Otherwise, `init_run_reconfigure: false` behaves as `init.reconfigure: never`.
- Otherwise, the legacy default `init_run_reconfigure: true` behaves as `init.reconfigure: auto` — **not**
  `always`. This is the one deliberate behavior change existing projects will see: Atmos no longer adds
  `-reconfigure` to every init by default. Projects that need the old unconditional behavior set
  `init.reconfigure: always` explicitly. Unlike `init.mode`/`init.upgrade` below, this reinterpretation is
  **not** rolled back by pinning an [edition](/cli/configuration/edition) — see [Editions](#editions).

### Editions

`init.mode` and `init.upgrade` are journaled in [`pkg/edition`](/cli/configuration/edition) (dated
2026-09-12, this PR): a project pinned to an edition from before that date gets `init.mode: always`
and `init.upgrade: never` restored automatically — byte-for-byte the behavior Atmos always had for
init and `-upgrade` — with no config changes. Unpinned projects, and anything pinned on or after
that date, get the new `auto` defaults. This is genuinely a new key each time (`init.mode` and
`init.upgrade` didn't exist before this PR), which per the editions system's own rule ("new
defaults are never journal-gated") would normally need no journal entry at all — but both keys
govern behavior Atmos always had a fixed, unconfigurable answer for before this PR (init always
ran; `-upgrade` was never passed automatically), so their ship-day default is treated as journaled
anyway, protecting existing projects from a silent behavior change on upgrade the same way a
changed default would be.

`init.reconfigure` cannot get the same treatment: unlike `init.mode`/`init.upgrade`, it has a
legacy predecessor (`init_run_reconfigure`) whose own fallback logic requires `init.reconfigure` to
stay genuinely unset when the user hasn't set it explicitly. Journaling it would mean giving it a
real Viper default, which would make it permanently non-empty and silently break
`init_run_reconfigure: false`'s `never` mapping for any project relying on it. The
`init_run_reconfigure: true` → `init.reconfigure: auto` reinterpretation this creates is real and
undocumented by the edition journal — it is the first `KindBehavior` candidate logged in
`docs/prd/editions.md`'s Roadmap after the editions system itself shipped, tracked there because
the editions system can gate a changed *value* (`KindValue`) but not yet a changed *meaning* of an
existing value (`KindBehavior`, reserved but unimplemented as of this PR).

---

## Skip Rule

### Fingerprint Inputs

Once Atmos has written the generated files for a component (varfile, `backend.tf.json`, provider overrides),
it computes a fingerprint over the inputs that actually affect what `terraform init` would do:

- Root-level `.tf`, `.tf.json`, `.tofu`, `.tofu.json` files in the component directory.
- `.terraform.lock.hcl`.
- The generated varfile and any `*.tfvars` files, **only when `init.pass_vars` is `true`** (init doesn't read
  vars otherwise, so they don't belong in the fingerprint when it's `false`).
- The Terraform CLI configuration content (the rendered `.terraformrc`/`TF_CLI_CONFIG_FILE`, when present).
- The resolved `terraform`/`tofu` binary (path and version).
- The `TF_CLI_ARGS`, `TF_CLI_ARGS_init`, `TF_PLUGIN_CACHE_DIR`, and `TF_DATA_DIR` environment variables.

### Structural Preconditions

A matching fingerprint is necessary but not sufficient — the working directory must also still look
initialized:

- Provider plugins are present when `.terraform.lock.hcl` lists providers.
- `.terraform/modules` is present when the configuration declares modules.
- Backend state is present when a backend is configured.

If the fingerprint matches but any structural precondition fails (for example, `.terraform` was partially
deleted by hand), Atmos treats it as a cache miss and re-initializes.

### Marker File

After each successful init, Atmos records the fingerprint in a marker file, `.terraform/atmos-init.json`
(written inside `TF_DATA_DIR` when the component sets one). The marker is a small JSON document:

```json
{
  "schema_version": 1,
  "fingerprint": "sha256:...",
  "backend_fingerprint": "sha256:...",
  "init_args": ["-reconfigure"],
  "atmos_version": "1.2xx.0",
  "binary": "terraform 1.9.x",
  "timestamp": "2026-09-11T12:34:56Z"
}
```

| Field | Purpose |
|---|---|
| `schema_version` | Bumped whenever the marker format changes; an unknown version is treated as a cache miss (forces re-init) rather than a parse error. |
| `fingerprint` | The full init-input fingerprint described above. Drives `init.mode: auto`'s skip/no-skip decision. |
| `backend_fingerprint` | A fingerprint over backend configuration alone (a subset of the inputs above). Drives `init.reconfigure: auto`'s decision independently of the broader skip decision — the backend can be unchanged while other init inputs changed, or vice versa. |
| `init_args` | The flags used for the init that produced this marker, kept for diagnostics (e.g. `atmos describe component` provenance, support requests). |
| `atmos_version` | The Atmos version that wrote the marker, for diagnosing behavior differences across upgrades. |
| `binary` | The resolved Terraform/OpenTofu binary and version, matching the fingerprint input. |
| `timestamp` | When the marker was written, for diagnostics only — not used in the skip decision. |

**Invalidation.** The marker is invalidated (treated as absent, forcing a fresh init) when:

- It is missing, unreadable, or fails to parse.
- `schema_version` doesn't match what the running Atmos binary expects.
- The fingerprint doesn't match the current inputs.
- A structural precondition (above) fails.

Deleting `.terraform` — including via [`atmos terraform clean`](/cli/commands/terraform/clean) — removes the
marker along with everything else, forcing a fresh init on the next command. An **explicit**
`atmos terraform init` invocation always runs, regardless of the marker, and refreshes it on success.

---

## Auto-Recovery Contract

`init.mode: auto` is a fingerprint-based heuristic, not a guarantee. Two situations can make a skipped init
wrong even though the fingerprint matched: something outside the fingerprint changed (see
[Known Gaps](#known-gaps)), or a human manually altered `.terraform` between commands. When that happens,
Terraform/OpenTofu itself detects the problem and fails with one of a small, closed set of diagnostics
**before touching state**:

| Diagnostic (Terraform/OpenTofu) | Meaning | Recovery |
|---|---|---|
| "Backend initialization required" | Backend block present but not initialized | Re-run init |
| "Required plugins are not installed" | A provider plugin is missing | Re-run init |
| "Module not installed" | A referenced module isn't in `.terraform/modules` | Re-run init |
| "Backend configuration changed" | Backend block differs from what's initialized | Re-run init with `-reconfigure` (if `init.reconfigure` is not `never`) |
| "...must use terraform init -upgrade" (provider version constraint changed) | Locked provider version no longer satisfies the constraint | Re-run init with `-upgrade` (if `init.upgrade` is not `never`) |

When Atmos recognizes one of these diagnostics on a command it ran with init skipped, it runs
`terraform init` (adding `-reconfigure`/`-upgrade` only when the diagnostic calls for it and the matching
setting isn't `never`) and **retries the original command exactly once**. All of these diagnostics are raised
during Terraform's own pre-flight checks, before any state read or write, which is what makes the retry safe:
worst case, Atmos does the init it should have done in the first place and re-runs the same command Terraform
never actually started.

**Opt-outs.** No retry happens when init was explicitly disabled for the invocation — `--skip-init`,
`init.mode: never`, or `deploy_run_init: false` for `deploy`. In that case the diagnostic surfaces as-is, with
an Atmos hint pointing at the setting that suppressed init. Likewise, if `init.upgrade: never` or
`init.reconfigure: never` is set and the diagnostic specifically calls for the flag that setting disables, the
diagnostic surfaces with a hint instead of being silently worked around — Atmos does not override an explicit
`never`.

**Atmos never runs `-migrate-state`** as part of auto-recovery, under any diagnostic. A backend change that
requires state migration is a decision a human should make explicitly.

---

## Behavior Matrix

| Command | Implicit init runs when | Adds `-reconfigure` when | Adds `-upgrade` when |
|---|---|---|---|
| `plan` | `init.mode` allows it (skip rule applies) | `init.reconfigure` allows it | `init.upgrade` allows it |
| `apply` | Same as `plan` | Same as `plan` | Same as `plan` |
| `deploy` | `deploy_run_init: true` **and** `init.mode` allows it | `init.reconfigure` allows it | `init.upgrade` allows it |
| `destroy` | Same as `plan` | Same as `plan` | Same as `plan` |
| `output` | Same as `plan` (the state-file path via `!terraform.state` needs no init at all) | Same as `plan` | Same as `plan` |
| `shell` | Same as `plan` | Same as `plan` | Same as `plan` |
| `workspace` | Always runs init (unchanged) | **Always**, regardless of `init.reconfigure` — computing/selecting the workspace requires a reconfigured backend | Same as `plan` |
| `init` (explicit) | Always runs — the skip rule never applies to an explicit `atmos terraform init` | Governed by `-reconfigure`/`init.reconfigure` like any other invocation, plus any native `-reconfigure` flag passed through | Governed the same way, plus any native `-upgrade` flag passed through |
| `plan-diff` | Same as `plan` for each side of the diff | Same as `plan` | Same as `plan` |
| `migrate` (`migrate plan`/`migrate apply`) | Same as `plan`; `--skip-init` is honored the same way | Same as `plan` | Same as `plan` |
| `!terraform.output` / `atmos.Component` | Same skip rule as `plan`, evaluated per referenced component | Same as `plan` | Same as `plan` |

---

## Interactions

- **`--skip-init`.** Unchanged: disables implicit init for that one invocation, equivalent to
  `--init-mode=never` for the run. Interacts with auto-recovery as described above (no retry).
- **`deploy_run_init`.** Unchanged: still the on/off switch for whether `deploy` runs init at all. When it's
  `true`, `init.mode` decides whether that init actually executes.
- **`--ui` streaming.** A skipped init produces no streaming init phase; the UI's phase list simply omits it
  for that run, the same way it already omits phases for commands that don't apply.
- **CI log groups.** A skipped init means no init log group is emitted for that invocation — one fewer group
  to expand, not an empty one.
- **`--dry-run`.** Dry-run evaluates the skip decision and reports it (would skip / would run, and why) without
  executing init or the underlying command.
- **`TF_DATA_DIR`.** The marker lives under `TF_DATA_DIR` when the component sets one, so alternate data
  directories get independent skip/init state, matching how Terraform itself scopes `.terraform` state.
- **JIT working directories** ([`provision.workdir`](/cli/commands/terraform/workdir)). Re-provisioning a JIT
  working directory always forces a fresh init with `-reconfigure` (per the `reconfigure: auto` rule above),
  because a freshly provisioned directory has no prior marker to match against.
- **`--all` / concurrency.** Multiple concurrent invocations against the same component (rare, but possible
  with `--max-concurrency` or overlapping automation) can race on writing the marker file. This is safe by
  design: the marker is last-writer-wins, and a wrong skip decision that results from the race is caught by
  the same [auto-recovery](#auto-recovery-contract) path that handles any other wrong skip — Terraform's own
  pre-flight diagnostics catch it before state is touched.
- **`atmos terraform clean`.** Removes `.terraform`, including the marker, forcing a fresh init on the next
  command — this is the documented way to force a full re-init without changing `init.mode`.

---

## Comparison

| | Atmos (this PRD) | Terragrunt Auto-Init | Terramate |
|---|---|---|---|
| Skip unchanged init | Yes, fingerprint-based | Yes, similar heuristic (source/backend hash) | No built-in auto-init; users script it |
| Adds `-reconfigure` conditionally | Yes (`auto`) | Partial — reconfigures on detected backend change | N/A |
| Adds `-upgrade` conditionally | Yes (`auto`), based on provider-constraint diagnostics | No — requires explicit `--terragrunt-source-update` or similar | N/A |
| Auto-recovers from a wrong skip | Yes, via a closed diagnostic table and one retry | Partial — re-runs init on specific known failures | N/A |
| Nested module changes tracked | No (known gap, shared with Terragrunt) | No (same limitation) | N/A |

Atmos's fingerprint has the same fundamental limitation Terragrunt's Auto-Init has: neither tool inspects the
contents of nested local modules for the purpose of deciding whether to re-init. Terramate does not provide
auto-init at all — it treats `terraform init` as a step the user's own script or Terramate script block must
invoke explicitly.

---

## Known Gaps

- **Nested local module changes are not fingerprinted.** Only root-level `.tf`/`.tf.json`/`.tofu`/`.tofu.json`
  files are part of the fingerprint. A change inside a module referenced via a relative path (`./modules/foo`)
  does not, by itself, invalidate the marker. In practice this is caught by [auto-recovery](#auto-recovery-contract)
  the moment Terraform notices the module needs re-installing or re-validating, but it means the first command
  after such a change pays for one extra init-and-retry cycle instead of Atmos catching it proactively. This
  mirrors Terragrunt's Auto-Init, which has the same limitation.

## Success Criteria

- Running `atmos terraform apply <component> -s <stack>` followed immediately by
  `atmos terraform output <component> -s <stack>` performs exactly one `terraform init` for the pair, not two.
- Bumping a provider version constraint and re-running `plan` triggers exactly one init with `-upgrade`
  (`init.upgrade: auto`), with no manual `-upgrade` flag needed. With `init.upgrade: never` set, the same
  scenario surfaces an explicit error with a hint instead of upgrading silently.
- A user who sets `init.mode: always` and `init.reconfigure: always` observes byte-for-byte the same init
  behavior Atmos had before this change — as does anyone pinned to an edition from before 2026-09-12, for
  `init.mode`/`init.upgrade` specifically, with no config changes at all.
- Deleting part of `.terraform` by hand, or a change inside a nested local module, is recovered automatically
  on the next command via the retry path — the user sees one extra init, not a hard failure.
- No test or documented workflow ever exercises `-migrate-state` as part of implicit or auto-recovered init.

---

## Release Checklist

### Blog Post

`website/blog/2026-09-11-smart-terraform-init.mdx`. Lead with the apply→output redundant-init example and
issue #620; cover the three settings, the `-upgrade` handling (issue #1263), and the `-reconfigure` behavior
change. Follow the `changelog` skill (`.claude/skills/changelog/SKILL.md`) for template, tags, and authors.

### Roadmap Entry

Add a shipped milestone to the `dx` (Developer Experience & Zero-Config) initiative in
`website/src/data/roadmap.js`, linking `changelog` to the blog slug. Follow the `roadmap` skill
(`.claude/skills/roadmap/SKILL.md`) — do not add to `featured[]` without explicit request.
