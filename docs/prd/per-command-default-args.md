# PRD: Per-Command Default Args

## Status

Shipped — initial implementation.

## Problem

Every Atmos flag's default is hard-coded in Go. Operators can't change a flag's default without
typing it on every invocation or exporting an environment variable. The intended precedence chain —
*CLI > ENV > config > default* — has a missing rung: there's no config-file way to say "for this
command, default `--process-functions` to `false`" or "disable `--identity` by default."

Users asked to disable identity/auth or YAML-function processing by default, per command, and to flip
those defaults per environment with [profiles](/cli/configuration/profiles).

## Decision: default *args*, not a typed map

An earlier iteration modeled this as a typed `defaults: {flag: value}` map per command. We rejected it
in favor of **default args** — raw, pass-through argument tokens — because the map is *not generic*: it
only covers flags Atmos models a specific way, and it needs per-command schema wiring as commands gain
new flags. Default args are "plain stupid" and future-proof: any flag, positional, repeated flag, or
future syntax works with **zero per-command wiring**.

```yaml
# global default args (every command)
args:
  - --identity=false

# per-command, path-derived
describe:
  args:
    - --process-functions=false
    - --skip=terraform.state
    - --skip=terraform.output

terraform:
  args:
    - --identity=false
```

`args` is the global list; `<command-path>.args` is per-command (`describe.args`,
`describe.component.args`, `terraform.args`, …), most-specific last. Note this is **path-derived by the
command line**, so `terraform.args` is a top-level key (the command `atmos terraform`), distinct from
`components.terraform` (the component-type config) — uniform addressing beats co-location.

## Goals

1. Set default arguments for any command in `atmos.yaml`, addressed by command path.
2. Uniform across **every** command with no per-command code.
3. Support repeatable flags and positionals naturally (raw tokens).
4. Overridable by profiles, environment variables, and the command line.

## Non-Goals

- A typed/validated per-flag schema (explicitly rejected — see above).
- Injecting args into custom commands' `steps` (separate subsystem).

## Architecture

A single argv preprocessor, run before Cobra parses, splices configured default args in for the target
command. This is uniform because it operates at the argv level — terraform/helmfile/packer (pass-through
flags), describe, list, validate, everything.

- **Config capture** (`pkg/config/load.go`): after the merged unmarshal (profiles applied),
  `atmosConfig.RawConfig = v.AllSettings()` stores the full settings map. The typed struct can't express
  arbitrary `<command>.args`, so the injector reads the raw map. New field
  `AtmosConfiguration.RawConfig` (`pkg/schema/schema.go`), never serialized.
- **Injector** (`pkg/flags/arg_defaults.go`): `InjectDefaultArgs(rootCmd, rawConfig, args)`:
  1. Resolves the target command via `rootCmd.Find` and its canonical path.
  2. `collectDefaultArgs` gathers `args` (global) + each path segment's `args`, least-specific first.
  3. `filterDefaultArgs` drops any default whose flag is already on the command line
     (`flagPresentInArgs`, up to a `--` separator) or whose env var is set (`flagEnvIsSet`: the
     `ATMOS_<UPPER_SNAKE>` convention plus any env vars declared on a global flag).
  4. Splices survivors in right after the command path, before the user's args.
  Help/completion paths are never mutated.
- **Wiring** (`cmd/root.go`): `preprocessArgs()` calls `flags.InjectDefaultArgs(RootCmd,
  atmosConfig.RawConfig, os.Args[1:])` as step 0, before NoOptDefVal and compat preprocessing.
  `atmosConfig` is already loaded at this point (`Execute` loads it before `preprocessArgs`), with
  profiles merged.

No changes to flag parsing, no viper default gymnastics — Cobra parses the injected args as if typed.

## Precedence

`CLI flag > ENV var > command default args > global default args > built-in default`.

- **CLI wins:** a default arg is skipped when its flag is already present (and injected args sit before
  the user's, so last-wins also favors the user).
- **ENV wins:** a default arg is skipped when its flag's env var is set. For an unrecognized raw token
  with no derivable env var, injection proceeds (nothing to honor).

## Profile compatibility

Structural: `RawConfig` is the post-profile-merge settings map, so a profile that sets any `*.args`
entry overrides the base. Profiles beat base config; CLI/ENV beat profiles.

## Verification

- Unit (`pkg/flags/arg_defaults_test.go`): path-derived collection/merge ordering, flag-name parsing,
  CLI-present and `--` pass-through detection, env suppression, splice position, subtree application,
  help no-op.
- Config layer (`pkg/config/arg_defaults_test.go`): merged settings expose global + per-command args;
  a profile overrides per-command args.
- Manual end-to-end (verified): `describe.args: [--process-functions=false]` leaves `!env USER`
  unprocessed; `--process-functions=true` (CLI) and `ATMOS_PROCESS_FUNCTIONS=true` (ENV) each override;
  a profile and `ATMOS_PROFILE` that set `--process-functions=true` flip it; global top-level `args:`
  applies across commands; absent config is a clean no-op.

## Follow-ups

- Let aliases carry default args (the same preprocessor makes this natural; aliases currently can't
  inject args).
- SchemaStore / editor autocomplete for `args:` keys.
- Optional typed `defaults: {flag: value}` sugar that compiles to args, for users who want validation.
