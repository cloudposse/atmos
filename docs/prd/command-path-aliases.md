# Command-Path Aliases with Default-Flag Injection

## Overview

`aliases:` in `atmos.yaml` supports multi-word command-path keys (e.g. `"terraform apply"`), not
just single-word shortcuts (e.g. `tf: terraform`). Same-name aliases — where the key is a prefix of
the value — behave like shell aliases and inject default flags into every invocation of that
command path. Expansion happens once, on raw argv, before Cobra parses the command line, so the
mechanism is uniform across every command and requires no per-command wiring.

## Problem Statement

Every CLI flag's default was hard-coded. There was no config-file way to:

- Disable identity/auth resolution by default for a command or subcommand path.
- Skip YAML-function processing by default for a specific `describe` invocation.
- Auto-approve a known-safe `terraform apply` path.
- Vary any of the above per environment or team, without editing wrapper scripts.

Aliases already existed as single-word shortcuts (`tf: terraform`), but they couldn't target a
subcommand path, and they had no way to carry default flags — only rename a command.

## Solution

### Configuration

```yaml
# atmos.yaml
aliases:
  # Shortcut aliases (existing behavior) — rename or abbreviate a command.
  tf: terraform
  tp: terraform plan
  up: terraform apply
  down: terraform destroy
  ds: describe stacks
  dc: describe component

  # Command-path aliases can add default flags. The key is quoted because it's
  # multi-word, and matches the full command path, not just the top-level command.
  terraform: terraform --identity=false
  "terraform apply": terraform apply -auto-approve
  "describe component": describe component --process-functions=false
```

A rule is a **default-injecting alias** when its value starts with its own key (e.g. `terraform` →
`terraform --identity=false`). A rule is a **shortcut/rewrite alias** when the value points to a
different command path (e.g. `tf` → `terraform`, `up` → `terraform apply`).

### Expansion mechanics (`cmd/internal/alias_expander.go`)

- `ConfigureCommandAliases` parses `schema.CommandAliases` into `aliasRule`s once at startup
  (`NewAliasExpander`), splitting each key/value into shell-quoted tokens (`parseAliasFields`, backed
  by `mvdan.cc/sh/v3/shell.Fields`).
- Rules are sorted by key length (longest first), then lexically, so a path alias like
  `"terraform apply"` is tried before the shorter `terraform` alias
  (`NewAliasExpander`'s `sort.SliceStable`). This is what lets a subcommand-specific default
  (`-auto-approve` only on `apply`) coexist with a command-wide default
  (`--identity=false` on all of `terraform`).
- `AliasExpander.Expand` repeatedly finds the longest matching prefix
  (`findLongestMatch`/`hasTokenPrefix`) and rewrites argv (`applyAliasRule`):
  - **Shortcut alias** (value doesn't start with key): argv's matched prefix is replaced wholesale
    by the value, and expansion loops again so shortcuts can chain (e.g. `up` → `terraform apply`,
    then a `"terraform apply"` default-injecting alias can still apply on top).
  - **Default-injecting alias** (value starts with key): only the flags *after* the shared prefix
    are spliced in, placed immediately after the command path and before the user's remaining args
    — then expansion stops (`hasTokenPrefix(rule.value, rule.key)` branch returns immediately,
    since re-matching would otherwise loop on the same command forever).
- A cycle guard (`seen` signatures) and a hard depth cap (`maxAliasExpansionDepth = 32`) turn
  accidental alias cycles into a clear `errAliasCycleDetected`/`errAliasExpansionExhausted` error
  instead of an infinite loop or stack overflow.

### Precedence: CLI > ENV > alias-injected default > built-in

Injected default flags must never silently override something the user actually set. Before
splicing in a default-injecting alias's flags, `suppressDefaultTokensFromEnv` drops any flag token
whose corresponding environment variable is already set — checked both as the conventional
`ATMOS_<FLAG_NAME>` form (`conventionalEnvVar`) and via each flag's registered env var aliases in
`flags.GlobalFlagsRegistry()` (`flagEnvIsSet`). CLI flags always win regardless, because injected
defaults are placed *before* the user's remaining argv tokens — Cobra/pflag's last-flag-wins parsing
means a later, explicit `--identity=true` overrides an earlier injected `--identity=false`:

```console
atmos terraform plan --identity=true
# aliases: { terraform: "terraform --identity=false" }
# expands to:
atmos terraform --identity=false plan --identity=true
# `--identity=true` wins because pflag applies flags in argv order.
```

### No changes to flag parsing

`ExpandCommandAliases` runs once on `os.Args`-derived argv before Cobra ever sees it
(`cmd/root.go`). Nothing in `pkg/flags/` changes — aliases are a pure argv-rewrite step, so the
mechanism works identically for every command (native and custom) with zero per-command
integration work.

## Alternatives Considered

### Alternative 1: Per-command default args via a new `args:` config section (original approach, rejected)

This PR originally implemented the same problem differently: a new `args:` (global) /
`<command-path>.args:` (per-command) config section in `atmos.yaml`, spliced onto argv by a
dedicated preprocessor (`flags.InjectDefaultArgs`, `pkg/flags/arg_defaults.go`), with precedence
`CLI > ENV > command default args > global default args > built-in`. It required capturing the
fully merged Viper settings as a new `AtmosConfiguration.RawConfig` map so `<path>.args` could be
looked up by dotted command path.

**Pros:**
- Explicit, single-purpose config key (`args:`) with no room for the "is this a shortcut or a
  default?" ambiguity that same-name aliases have.
- No dependency on the pre-existing `aliases:` section's shape or matching rules.

**Cons:**
- Introduced a **second, parallel config surface** doing almost the same job as the existing
  `aliases:` shortcut mechanism (both rewrite argv before parsing), so users had two places to look
  for "how do I change what a command does by default."
- Required a new cross-cutting capture on `AtmosConfiguration` (`RawConfig`) purely to support
  dotted-path lookup, adding surface area with no other consumer.
- Didn't compose with aliases users already understood (`tf: terraform`) — defaults and shortcuts
  were unrelated concepts even though both are "rewrite argv for this command."

**Decision:** Rejected (commit `8a1d7b268a`, "Replace default args with command aliases") in favor
of folding default-flag injection into the existing `aliases:` mechanism via same-name command-path
rules. One config section now covers shortcuts, command-path targeting, and defaults.

### Alternative 2: Typed `flag: value` map per command

Instead of raw argv tokens, define defaults as a typed map, e.g.:

```yaml
defaults:
  terraform:
    identity: false
  "terraform apply":
    auto-approve: true
```

**Pros:**
- Schema-validated; typos in flag names could be caught at config-load time.
- No shell-token parsing required.

**Cons:**
- Requires a schema entry and validation path per flag, per command — new commands/flags need new
  schema work to become configurable this way.
- Doesn't support positional arguments, repeated flags (e.g. multiple `--skip`), or flag syntaxes
  that don't map cleanly to a single scalar/bool (e.g. `--var foo=bar` repeated).
- One typed layer per command is exactly the kind of per-command wiring both the default-args and
  alias approaches were trying to avoid.

**Decision:** Rejected in the original PRD's rationale and not revisited during the pivot to
aliases; raw argv tokens (`shell.Fields`-parsed strings) stay future-proof as commands gain flags,
at the cost of losing compile-time/schema validation of individual flag names.

## References

- Docs: [`/cli/configuration/aliases`](../../website/docs/cli/configuration/aliases.mdx)
- Blog: [`2026-07-04-command-path-aliases`](../../website/blog/2026-07-04-command-path-aliases.mdx)
- Implementation: `cmd/internal/alias_expander.go`
- Tests: `cmd/internal/alias_expander_test.go`
- Roadmap: `website/src/data/roadmap.js` (PR #2572, changelog `command-path-aliases`)
- Related, unrelated PRD: [`command-alias-pattern.md`](./command-alias-pattern.md) describes a
  different feature (declarative `CommandProvider.GetAliasTarget()` command-forwarding for
  Go-defined commands like `toolchain search` → `toolchain registry search`), shipped separately in
  PR #1686. That mechanism operates on the Cobra command tree at registration time; this PRD's
  mechanism operates on raw argv before Cobra parses anything. The two are complementary, not
  overlapping.
