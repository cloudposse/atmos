---
name: atmos-vhs
description: >-
  Reusing existing VHS-dialect .tape files with Atmos: the type: cast step's tape:/tape_file:
  fields that interpret VHS tape syntax in memory (no YAML generated, nothing written to disk),
  the atmos cast record <input.tape> CLI entry point for one-off tapes with no workflow YAML,
  which VHS directives map to real cast fields vs. are cosmetic-ignored vs. force mode: session,
  and how to choose (or hand-migrate to) mode: steps for real per-command exit codes vs.
  mode: session for raw keypress/PTY fidelity.
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/vhs-directive-support.md
---

# Atmos VHS

[VHS](https://github.com/charmbracelet/vhs) is an external tool with its own `.tape` scripting
DSL for recording terminal sessions. Many Atmos users already have `.tape` files -- often written
for the real `vhs` binary, sometimes just copied from a demo repo. This skill covers Atmos's own
**subset-compatible interpreter** of that same tape syntax, built into the `type: cast` workflow
step: it parses and executes VHS-dialect directives directly, in memory, every run. It is not a
wrapper around the real `vhs` binary -- no VHS/`ttyd`/`ffmpeg` install required, and no `.tape` is
ever converted into generated YAML on disk.

## Related Skills

| Need | Load |
|---|---|
| `type: cast` step mechanics: `mode: steps`/`mode: session`, output formats, `type: simulate` | [atmos-cast](../atmos-cast/SKILL.md) |
| Embedding a cast step inside a larger multi-step workflow | [atmos-workflows](../atmos-workflows/SKILL.md) |

Quick recap (full detail lives in `atmos-cast`): `mode: steps` runs real nested steps and records
them, with full exit-code propagation. `mode: session` drives one PTY-backed shell through
scripted `write`/`key`/`pause`/`wait` actions -- realistic keypress fidelity, but no per-command
exit codes. Everything below is about which of those two modes a given VHS tape needs, and how far
you can get without hand-writing either.

## Two Paths: Interpret or Hand-Migrate

1. **Interpret as-is** -- point `tape:`/`tape_file:` (or `atmos cast record`) straight at an
    existing `.tape` file. Zero rewrite for tapes that only use directives in the "mapped" or
    "cosmetic" categories below. This is the fastest path and the right default.
2. **Hand-migrate to native `steps:`/session actions** -- rewrite the tape's on-camera commands as
    `type: simulate` + `type: shell`/`type: atmos` pairs (`mode: steps`) or `write`/`wait` session
    actions (`mode: session`). Do this when you want per-step names for observability, want to
    interleave real Atmos-native step types the tape can't express (structured error handling,
    `type: require`, `type: toast`), or when a tape mixes `mode: steps`-legal and session-only
    directives and you want to keep the steps-legal parts under `mode: steps` deliberately.

Both paths can coexist: interpret a tape unmodified today, hand-migrate later only if you outgrow
it.

## `tape:` / `tape_file:` and `atmos cast record`

```yaml
steps:
  - type: cast
    name: quick-demo
    mode: session
    tape_file: demo/landing/hero.tape   # path to an external .tape file
```

```yaml
steps:
  - type: cast
    name: inline-demo
    mode: session
    tape: |
      Require atmos
      Set Shell bash
      Set WaitTimeout 30s
      Type "atmos version" Enter
      Wait /❯$/
```

An explicit step-level YAML field (`shell:`, `width:`, `height:`, `output:`, and so on) always
wins over the value a tape directive would otherwise imply -- the tape fills in gaps, it never
overrides what you set explicitly in YAML.

For a single tape with no workflow YAML at all, use the CLI directly -- it reuses the exact same
interpreter as `tape_file:`, just without a `type: cast` step wrapping it:

```shell
atmos cast record demo/landing/hero.tape --output=hero.mp4          # mode: session (default)
atmos cast record demo.tape --output=demo.cast --mode=steps         # real exit codes
```

`hero.tape` itself only works under `mode: session` -- its `Hide` block (see the worked example
below) is a hard parse-time error under `mode: steps` regardless of what's inside it. `--mode=steps`
only applies to a tape with no `Hide`/`Show`, bare keypresses, or `Type` without a trailing `Enter`
(see "Choosing `mode: steps` vs `mode: session`" below).

`atmos cast record` is a distinct subcommand from `atmos cast play`/`atmos cast render` -- `play`
and `render` are unchanged and still only operate on `.cast` recordings; neither accepts a tape,
since a tape isn't a cast.

## Choosing `mode: steps` vs `mode: session` for a Tape

This is the decision that matters most. Scan the tape for any of these three things:

- [ ] A **bare/standalone keypress** anywhere: `Enter`, `Space`, `Tab`, `Up`, `Down`, `Left`,
      `Right`, `Backspace`, `Escape`, `PageUp`, `PageDown`, `Ctrl+<letter>` -- when it is *not*
      immediately the terminating `Enter` of a `Type "..." Enter` line.
- [ ] A `Hide` ... `Show` block anywhere, regardless of what it contains.
- [ ] A `Type "text"` with **no** trailing `Enter` -- a bare keystroke sent to an already-running
      interactive program.

**None found** -> `mode: steps` is legal. Use it: you get real, individually tracked exit codes
for every command, which is the entire point of choosing `steps` for a demo that should actually
prove something ran correctly.

**Any found** -> `mode: session` is required. Trying to interpret that tape under `mode: steps`
is a **hard parse-time error** naming the offending line and telling you to switch to
`mode: session`.

`demo/landing/dx.tape` is a real example that forces `mode: session`:

```text
Type "atmos terraform deploy" Sleep 600ms Enter
Sleep 2.5s
Down
Sleep 700ms
Enter
Sleep 1.5s
Enter
Wait /❯$/
```

The `Down` / `Enter` / `Enter` sequence drives an interactive plan-confirmation picker with raw
keypresses -- there is no discrete "step" that means "press the down arrow once." That tape can
only be interpreted (or hand-migrated) under `mode: session`.

The important nuance: under `mode: session`, `Hide`/`Show` around plain `export`/`unset`/`cd`/
`clear` lines gets lifted automatically -- Atmos steps already have native `env:` and
`working_directory:` fields, `Hide` in VHS is purely a workaround for not having those, and the
session translator recognizes this common case and turns it into step config instead of literal
PTY replay. `demo/landing/hero.tape`'s `Hide` block is exactly this case (see the worked example
below): four lines, all `unset`/`export`/`cd`/`clear`. But that lift is `mode: session`-only --
under `mode: steps` there's no PTY pass over the tape to look inside a `Hide` block in the first
place, so *any* `Hide`/`Show` token, regardless of contents, is still the hard parse-time error
described above. Getting the steps-mode equivalent of `hero.tape`'s `Hide` block requires the
manual hand-migration shown in worked example (b) below, not the automatic interpreter.

## Worked Example: `demo/landing/hero.tape`

`demo/landing/hero.tape` is a real VHS tape in this repo, normally driven by the actual `vhs`
binary via the separate `atmos demo record` pipeline (unrelated to `type: cast`). It sources
`defaults.tape`, hides a setup block that only exports env vars and `cd`s into a fixture, types
two on-camera commands, and takes a trailing `Screenshot`:

```text
Source demo/landing/defaults.tape

Output website/static/img/demos/hero.webm
Output website/static/img/demos/hero.mp4

Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ...` Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/hero"` Enter
Type "clear" Enter
Wait /❯$/
Show

Sleep 800ms
Type "atmos list stacks" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos list components -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/hero.png
Sleep 5s
```

### (a) Interpret as-is via `tape_file:`

```yaml
steps:
  - type: cast
    name: hero-demo
    mode: session
    tape_file: demo/landing/hero.tape
```

**This does not work unmodified.** The first `Output ... hero.webm` line is a hard error --
`.webm` is not one of the extensions the interpreter (or `atmos cast render`) accepts. The fix is
a one-line edit: delete that line, or change it to `.mp4` (the very next `Output` line already
targets `.mp4`, so deleting the `.webm` line is enough). Every other directive in this tape --
`Source`, the `Hide`/`Show` block, `Sleep`/`Wait`, `Screenshot` -- interprets unmodified.

### (b) Hand-migrated to `mode: steps`

The `Hide` block is only `unset`/`export`/`cd`/`clear`, so it lifts to `env:`/`working_directory:`
and the tape's on-camera `Type "cmd" Enter` lines each become a `type: simulate` (so the recording
still looks hand-typed) paired with a real `type: shell` (so it actually runs, with a real exit
code) -- the same `simulate` + real-step pairing already used in
`demo/casts/atmos.d/examples/quick-start-simple/list-and-plan.yaml`, keeping the simulated text
and the real command in sync with a YAML anchor:

```yaml
steps:
  - type: cast
    name: hero-demo-steps
    mode: steps
    working_directory: demo/landing/fixtures/hero   # lifted from the Hide block's `cd` line
    env:
      ATMOS_EXPERIMENTAL: "silence"
      ATMOS_TERMINAL_SPEED: "8"
      # ...rest of the Hide block's `export` line, lifted the same way;
      # the `unset NO_COLOR ATMOS_NO_COLOR` line lifts too, clearing those vars from env:
    output:
      cast: website/static/img/demos/hero.cast
    steps:
      - type: simulate
        mode: typed
        text: &list_stacks_cmd atmos list stacks
      - type: shell
        command: *list_stacks_cmd
      - type: simulate
        mode: typed
        text: &list_components_cmd atmos list components -s dev
      - type: shell
        command: *list_components_cmd
```

`Sleep`/`Wait` lines are simply dropped -- step execution is already synchronous, there's nothing
to wait for. Note what's lost: there's no native `steps:` field yet for a mid-recording,
point-in-time `Screenshot` capture like the tape's trailing line -- that's a good reason to prefer
interpreting the tape (path (a)) instead of hand-migrating it, specifically when it uses
`Screenshot`.

### (c) Hand-migrated to `mode: session` (session-faithful)

If you want to keep PTY fidelity instead (e.g. this tape also had raw keypresses), the same
on-camera section becomes scripted session actions:

```yaml
steps:
  - type: cast
    name: hero-demo-session
    mode: session
    shell: bash
    working_directory: demo/landing/fixtures/hero
    output:
      mp4: website/static/img/demos/hero.mp4
    steps:
      - type: write
        text: "atmos list stacks\n"
      - type: wait
        regex: "❯\\s*$"
        timeout: 30s
      - type: write
        text: "atmos list components -s dev\n"
      - type: wait
        regex: "❯\\s*$"
        timeout: 30s
```

## Directive Support at a Glance

| Category | Directives | Behavior |
|---|---|---|
| Mapped to a real cast field | `Set Shell`, `Set Width`/`Height`, `Set TypingSpeed`, `Set WaitTimeout`, `Output`, `Require`, `Source` | Fills in the corresponding step field/behavior; explicit YAML always wins |
| Cosmetic -- warns, then ignored | `Set FontFamily`/`FontSize`/`Theme`/`Margin`/`Padding`/`BorderRadius`/`Framerate`/`CursorBlink`/`LetterSpacing`/`LineHeight`/`MarginFill`/`WindowBar`/`PlaybackSpeed`/`LoopOffset`, `Set WaitPattern` (v1) | No cast equivalent; logs a warning naming the key and continues without applying it |
| Session-only | bare keypresses, `Type` with no `Enter`, `Hide`/`Show` (always, regardless of contents) | Legal under `mode: session` only; hard parse-time error under `mode: steps` |
| Pacing, dropped under `mode: steps` | `Sleep`, `Wait` | Under `mode: session`, translated to real `pause`/`wait` actions; under `mode: steps`, step execution is already synchronous, so these are dropped with a warning (not an error) |
| Works in both modes | `Screenshot <path>` | Marks a still-image capture point in the recording |
| Always a hard error | `Copy`, `Paste`, `Env` | Not silently dropped -- rewrite as `Type`/`export` (see reference) |

For the exhaustive, directive-by-directive table -- every `Set <Key>`, the full `Output`
extension list, every keypress name, and rewrite examples for `Copy`/`Paste`/`Env` -- read
[references/vhs-directive-support.md](references/vhs-directive-support.md).

## `Wait` vs `Wait+Screen`: the Scrolling Trap

Prefer a bare `Wait /pattern/` over `Wait+Screen /pattern/` for any command whose output can
scroll (`terraform plan`/`apply`/`deploy`, long `list`/`describe` output). This mirrors a real,
cited VHS bug ([charmbracelet/vhs#657](https://github.com/charmbracelet/vhs/issues/657) /
[#659](https://github.com/charmbracelet/vhs/issues/659), documented in
`demo/landing/README.md`): once terminal output scrolls, `Wait+Screen` (and `.ascii`/`.txt`
capture) search the *top* of scrollback instead of the visible viewport, so the match can time out
even though the recording itself is fine. The interpreter is a from-scratch implementation, not a
call into the real `vhs` binary, but it reproduces the same terminal-cell-grid model -- so the
same caveat applies. Anchor `Wait` on the prompt returning (e.g. `Wait /❯$/`) rather than trying to
match specific output text for anything long-running.

## `Source`

`Source <file>` inlines another tape file's directives at that point, resolved relative to the
step's `working_directory` (or the process CWD if unset) -- the same fixed base `tape_file` itself
resolves against, and the same base every `Source` uses no matter how many levels deep. This
matches real `vhs`: `vhs script.tape` resolves every `Source` in the script relative to its own
invocation directory, not relative to whichever file a `Source` happens to be read from. Concretely,
`hero.tape`'s own `Source demo/landing/defaults.tape` line is written as a path relative to the repo
root (where `vhs` is invoked from), even though `hero.tape` itself already lives in
`demo/landing/` -- resolving it relative to `hero.tape`'s own directory would incorrectly look for
`demo/landing/demo/landing/defaults.tape`. That `Source` line pulls in `defaults.tape`'s
`Require atmos`, `Set Shell bash`, `Set WaitTimeout 300s`, and every cosmetic `Set` before
`hero.tape`'s own `Output`/`Hide`/`Show` content runs. `Source` chains -- a sourced file can itself
`Source` another file -- and is commonly used the way `demo/landing/*.tape` uses it: one shared
`defaults.tape` for look-and-feel and wait behavior, sourced by every topical tape.

## `Require`

`Require <program>` maps onto the same tool-presence check as Atmos's own `type: require` step
(`tools: [program]`) -- it fails fast, before anything runs, if the named program isn't on `PATH`.
`defaults.tape`'s `Require atmos` line becomes an implicit `tools: [atmos]` precondition on the
cast step.

## Common Patterns

- Try interpreting a tape unmodified first (`tape_file:` or `atmos cast record`) before hand-
  migrating anything -- most tapes that don't drive an interactive picker or use `Hide` for
  something exotic work with zero rewrite.
- Use `atmos cast record <tape> --output=<path>` as a fast smoke test for "does this tape even
  work under the interpreter" before wiring it into a `type: cast` step inside a workflow.
- Keep a tape's `Hide` block to `export`/`unset`/`cd`/`clear` only if you want it to stay portable
  across both `mode: steps` and `mode: session` -- anything else (typing a partial command,
  toggling settings mid-`Hide`) locks the tape into `mode: session`.
- Delete or rewrite any `Copy`/`Paste`/`Env` line before pointing the interpreter at a tape
  authored for the real `vhs` binary -- they always error, on purpose, rather than silently
  producing a subtly broken demo.
- Fix `Output ... .webm` lines (common in tapes written for `atmos demo record`, which targets
  `.webm`/`.mp4` for the website) to a supported extension (`.mp4` is usually already present
  alongside it) before interpreting a tape that was written for the real `vhs` binary.

## Reference Files

- [references/vhs-directive-support.md](references/vhs-directive-support.md) -- exhaustive
  directive-by-directive support matrix: every `Set <Key>`, the full `Output` extension
  acceptance list, every keypress name, the exact `mode: steps` error-trigger list, and
  `Copy`/`Paste`/`Env` rewrite examples.
