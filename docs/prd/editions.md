# Editions: Date-Anchored Defaults

## Overview

Atmos defaults change over time. Before editions, every such change silently altered behavior for
every user on upgrade — and, worse, some declared changes never took effect because Atmos has
multiple default layers that drifted apart. Editions journal every change to a previously shipped
default with a date, and let users anchor ("pin") a project to a date so upgrading the Atmos binary
never silently changes its defaults.

The model follows Rust editions (and Cloudflare Workers' `compatibility_date`): the pin names a
point in the project's relationship with the tool, not a version of the tool.

```yaml
# atmos.yaml (top-level key)
edition: "2026-01"   # or "2026" (whole year) or "2026-01-15" (exact day)
```

Also available as the `--edition` global flag and the `ATMOS_EDITION` env var
(precedence: flag > env > config).

## Semantics

- **No pin → latest defaults.** The overlay is empty; behavior is byte-for-byte what it was before
  this feature existed. Zero breaking change.
- **Pinned at date D:** for each config key, the earliest journal entry dated **after** D (if any)
  supplies its `Old` value as the effective default. Entries dated **on or before** D apply their
  `New` value (the pin date is inclusive). Chained changes (A→B→C) resolve to the value the project
  actually saw at D.
- **Partial dates round to the END of the period**: `"2026"` → 2026-12-31, `"2026-07"` → 2026-07-31.
  "The 2026 edition" includes everything shipped during 2026, matching Rust's mental model.
- **New defaults are never journal-gated.** A newly introduced key supersedes nothing, so a new
  feature always loads with its initial default regardless of the pin. Only *changes* to previously
  shipped defaults enter the journal.
- **User-set values always win.** The overlay is applied via Viper's defaults layer
  (`SetDefault`), which every config source (file, `atmos.d`, profile, env var, flag) outranks.
- **The `edition` key itself is permanently exempt from journaling** (test-enforced); otherwise a
  pin could alter what pinning means.
- **Experimental:** a pinned project is gated through `settings.experimental`
  (silence/disable/warn/error), like `settings.yaml.key_delimiter`.

## Journal contents (v1)

The journal ships with 12 entries reaching back to February 2025 (source of truth:
`pkg/edition/journal.go`):

| Date | Key | Old | New | Ref |
|---|---|---|---|---|
| 2025-02-11 | `logs.file` | `/dev/stdout` | `/dev/stderr` | [#1050](https://github.com/cloudposse/atmos/pull/1050) |
| 2025-09-23 | `logs.level` | `Info` | `Warning` | [#1430](https://github.com/cloudposse/atmos/pull/1430) — newly effective in this PR (see drift section) |
| 2025-10-16 | `settings.terminal.pager` | `"true"` | `"false"` | [#1642](https://github.com/cloudposse/atmos/pull/1642) |
| 2025-12-06 | `stacks.inherit.metadata` | `false` | `true` | [changelog/metadata-inheritance](https://atmos.tools/changelog/metadata-inheritance) |
| 2026-02-10 | `components.helmfile.use_eks` | `true` | `false` | [#1903](https://github.com/cloudposse/atmos/pull/1903) |
| 2026-07-06 | `settings.terminal.help.filter` | `false` | `true` | [#2696](https://github.com/cloudposse/atmos/pull/2696) |
| 2026-07-13 | `describe.error_mode` | `strict` | `warn` | [changelog/list-describe-graceful-degradation](https://atmos.tools/changelog/list-describe-graceful-degradation) |
| 2026-07-13 | `list.error_mode` | `strict` | `warn` | [changelog/list-describe-graceful-degradation](https://atmos.tools/changelog/list-describe-graceful-degradation) |
| 2026-07-17 | `describe.component.filter` | `full` | `schema` | this PR ([changelog/config-editions](https://atmos.tools/changelog/config-editions)) |
| 2026-07-16 | `describe.provenance` | `false` | `true` | this PR ([changelog/config-editions](https://atmos.tools/changelog/config-editions)) |
| 2026-07-16 | `list.instances.format` | `"table"` | `"tree"` | this PR ([changelog/config-editions](https://atmos.tools/changelog/config-editions)) |
| 2026-07-16 | `stacks.list.format` | `"table"` | `"tree"` | this PR ([changelog/config-editions](https://atmos.tools/changelog/config-editions)) |

The four `2026-07-16` and de-shadowed `logs.level` defaults become newly effective for
un-pinned projects in this PR: log level `Info` → `Warning`, provenance annotations on by
default in `describe component`, and `tree` as the default format for `list stacks` /
`list instances`.

## Architecture

### The four default layers

All defaults funnel through `LoadConfig` (`pkg/config/load.go`). A default can originate in four
places, and understanding them is essential because the journal can only roll back one of them:

| Layer | Where | Viper layer | Journal coverage |
|---|---|---|---|
| (a) `setDefaultConfiguration` | `pkg/config/load.go` | defaults (`SetDefault`) | **v1 target** |
| (b) embedded `atmos.yaml` | `pkg/config/atmos.yaml` (go:embed) | config (`MergeConfig`) | must never contain journaled keys (test-enforced) |
| (c) `defaultCliConfig` struct | `pkg/config/default.go` | config; only when **no** atmos.yaml exists | must agree with each entry's `New` (test-enforced) |
| (d) flag defaults | `pkg/flags/` | defaults, on the global Viper | deferred to v2 |

Layers (b) and (c) merge into Viper's **config layer**, which the defaults layer cannot override.
That is why the invariant tests in `pkg/config/edition_invariants_test.go` pin down two policies:
journaled keys must be absent from the embedded atmos.yaml, and `defaultCliConfig` must state the
same current value as the journal (or omit the field).

### Components

- **`pkg/edition/journal.go`** — the append-only journal: `Entry{Date, Key, Kind, Old, New,
  Description, Ref}`. Compile-time Go entries (not a data file) so values stay typed, the entry is
  code-review-adjacent to the default change, and tests can cross-check the journal against live
  defaults in-process. `Kind` is `value` today; `behavior` is reserved (see Roadmap).
- **`pkg/edition/anchor.go`** — `ParseAnchor` parses `YYYY[-MM[-DD]]` and rounds partial dates to
  the end of the period.
- **`pkg/edition/resolve.go`** — `Overrides(anchor)` (key → old default to re-apply),
  `Diff(from, to)` (effective-default changes between any two anchors; `nil` = latest, so
  `Diff(pin, nil)` answers "what changes if I unpin?"), and `Between(from, to)` (journal entries in
  a window, used by `atmos list editions`).
- **`pkg/edition/describe.go`** — `DescribePin` powers `atmos describe edition`.
- **`pkg/config/edition.go`** — `applyEditionDefaults(v)`: resolves the pin (global-Viper flag with
  an os.Args fallback for `DisableFlagParsing` commands → `ATMOS_EDITION` → `edition:` key), applies
  `Overrides` via `SetDefault`, and records the winning pin back into the local Viper so
  `atmosConfig.Edition` reflects it. Called in `LoadConfig` immediately before the final
  `Unmarshal`, after config files, `atmos.d`, profiles, and env bindings have merged (so a profile
  may set the pin), and in the `loadConfigFromCLIArgs` early-return path.

### CLI surface

- `atmos list editions` — renders the journal newest-first; `--from`/`--to` anchors narrow it to
  the changes between two editions (the edition diff); `--format` supports the standard list
  formats.
- `atmos describe edition` — the active pin: raw value, resolved date, granularity, source
  (flag/env/config), and each rolled-back default with its pinned and latest values.

## Guardrails

Two test layers make an unjournaled default change un-mergeable:

1. **Snapshot** (`pkg/config/default_snapshot_test.go` +
    `pkg/config/testdata/default-config-snapshot.yaml`): flattens `setDefaultConfiguration` and
    compares against the committed golden file.
    - Changed value → requires a journal entry whose `Old`/`New` match, **and** snapshot
      regeneration.
    - New key → snapshot regeneration only (encodes the new-defaults-aren't-gated rule).
    - Removed key → fails; removals are out of the journal's scope and need explicit regeneration.
    - Regenerate: `ATMOS_REGENERATE_DEFAULTS_SNAPSHOT=true go test ./pkg/config -run TestDefaultConfigurationSnapshot`
    - `components.terraform.append_user_agent` is exempt (embeds the build version).
2. **Invariants** (`pkg/edition/journal_invariants_test.go` and
    `pkg/config/edition_invariants_test.go`): dates valid and chronological per key; chains
    consistent (`entry[n+1].Old == entry[n].New`); each key's newest `New` equals the live default;
    journaled keys absent from the embedded atmos.yaml; `defaultCliConfig` agreement; the `edition`
    key never journaled.

### Contributor workflow

Changing a shipped default is now three edits in one PR:

1. Change the literal in `setDefaultConfiguration` (`pkg/config/load.go`).
2. Append a dated `Entry` to `pkg/edition/journal.go` (date = expected merge date, `Ref` = PR URL,
    `Old`/`New` in the type the field uses today).
3. Regenerate the snapshot.

Adding a brand-new default needs only edits 1 and 3.

## Historical drift this feature surfaced (and fixed)

Building the journal required 12 months of git archaeology over the default layers, which exposed
three cases where a declared default change never (fully) took effect because the layers disagreed:

1. **`components.helmfile.use_eks`** — PR #1903 (2026-02-10) flipped the struct default to `false`
    but left layer (a) at `true`, so every project with an atmos.yaml kept the old behavior. Fixed
    here (layer (a) now `false`) and journaled, so `edition: "2026-01"` genuinely restores `true`.
2. **`settings.terminal.pager`** — PR #1642 (2025-10-16) disabled the pager in layer (a), but
    `defaultCliConfig` still said `"less"`, so no-config-file projects kept a pager. Aligned to
    `"false"` here and journaled.
3. **`logs.level`** — PR #1430 declared `Info` → `Warning` in layers (a) and (c), but the embedded
    atmos.yaml (layer b) set `logs.level: Info` in the config layer, which wins — so the effective
    default remained `Info` for over nine months after the change was declared. **Resolved in this
    PR:** the embedded atmos.yaml no longer sets any logging keys (de-shadowed; the journaled-keys-
    absent invariant test now enforces this), making `Warning` genuinely effective for un-pinned
    projects, and the change is journaled (dated `2025-09-23`, when PR #1430 declared it, so pins
    from before that date restore `Info`).

The pager entry also set the precedent that `Old`/`New` are recorded in the type the field uses
**today** (string `"true"`/`"false"`), because `Old` is re-injected via `SetDefault` against the
current schema.

## Testing

- `pkg/edition/*_test.go` — anchor rounding (incl. leap February), invalid formats, chained-change
  resolution, inclusive anchor dates, diff in both directions and between windows, journal isolation
  (returned copy), self-contained journal invariants.
- `pkg/config/load_edition_test.go` — end-to-end through `LoadConfig`: pin restores old defaults,
  explicit config beats pin, env pin, env-beats-config precedence, invalid pin errors, no pin is
  byte-identical to before.
- `cmd/list/editions_test.go`, `cmd/describe_edition_test.go` — command-level helpers.

## Roadmap (v2+)

- **Flag defaults (layer d):** a post-LoadConfig `SetDefault` pass over the global Viper; the
  `Entry.Key` namespace already accommodates it.
- **Behavior gating:** `KindBehavior` entries with an `edition.BehaviorChanged(cfg, key)` predicate
  so code paths (not just values) can branch on the pin — what makes editions Rust-like rather than
  value snapshots. The journal type and resolution engine were designed so this is purely additive.
  A sweep of the ~18 months before this feature shipped mined these behavior-change candidates as
  seed entries (none is expressible as a value rollback today):
  - 2025-10-27 — Atmos adopted macOS XDG CLI path conventions, relocating config/cache/state
    directories on macOS —
    [changelog/macos-xdg-cli-conventions](https://atmos.tools/changelog/macos-xdg-cli-conventions).
  - 2025-11-16 — `atmos auth logout` flipped from always deleting keyring credentials to preserving
    them by default (deletion moved behind the new `--keychain` flag) —
    [PR #1791](https://github.com/cloudposse/atmos/pull/1791).
  - 2026-01-06 — `base_path` discovery changed to anchor at the git repository root —
    [changelog/base-path-behavior-change](https://atmos.tools/changelog/base-path-behavior-change)
    ([PR #1872](https://github.com/cloudposse/atmos/pull/1872),
    [PR #1868](https://github.com/cloudposse/atmos/pull/1868)).
  - 2026-01-10 — stack references collapsed to a single canonical stack identifier.
  - 2026-01-29 — `--chdir` gained config isolation, no longer leaking the caller's config into the
    target directory.
  - 2026-03-09 / 2026-05-23 — `terraform --all` executes components in dependency order and no
    longer requires `-s` — [PR #1516](https://github.com/cloudposse/atmos/pull/1516).
  - 2026-04-29 — `describe affected` emits matrix output automatically in CI.
  - 2026-07-05 — `atmos git clone` gained a fork-PR safety gate that blocks untrusted fork remotes
    by default.
  - 2026-07-10 — interactive workflow/custom-command steps in non-TTY environments flipped from
    erroring to falling back to the step's configured `default` value —
    [PR #2714](https://github.com/cloudposse/atmos/pull/2714).

  **Not gatable:** the auth credential realm isolation change (2026-02-10,
  [changelog/auth-realm-isolation](https://atmos.tools/changelog/auth-realm-isolation)) is a hard
  break — cached credentials moved realms and every user had to re-login. Editions cannot roll it
  back; it is documented here so nobody expects a `KindBehavior` entry to cover it.
- **Layer (c) coverage** if the env-pin-without-config-file edge proves real.
- **Graduation out of experimental** once the journal has accumulated real-world use.

## References

- Implementation PR: this PR.
- Rust editions: https://doc.rust-lang.org/edition-guide/
- Cloudflare Workers compatibility dates: https://developers.cloudflare.com/workers/configuration/compatibility-dates/
- Prior art in-repo: `docs/prd/experimental-features-system.md`, `docs/prd/version-constraint.md`,
  `docs/prd/helmfile-use-eks-default-change.md` (the migration doc that is effectively the first
  journal entry in prose form).

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-07-16 | 1.0 | Initial PRD; v1 implementation (value defaults, journal, pin, list/describe commands, guardrails). |
