---
name: atmos-cast
description: "Atmos cast recording and rendering: atmos cast play/render, output-format inference (gif/mp4/html/ascii/png/jpg/jpeg), the --cast/ATMOS_CAST auto-recording flag, workflow type: cast (steps/session modes) and type: simulate step types, and committed .cast screengrabs"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Cast

Use this skill for Atmos's built-in terminal session recording and rendering subsystem. A cast is
an asciicast v2 recording (`.cast`) of a terminal session that Atmos can play back or render into
shareable artifacts (GIF, MP4, HTML, ASCII, PNG, JPEG).

## Related Skills

| Need | Load |
|---|---|
| Workflow step types in general, orchestration, output/UI steps | [atmos-workflows](../atmos-workflows/SKILL.md) |
| Custom CLI commands with native steps | [atmos-custom-commands](../atmos-custom-commands/SKILL.md) |
| Global settings (`atmos.yaml` non-subsystem options) | [atmos-settings](../atmos-settings/SKILL.md) |

## Purpose

Cast recording turns a terminal session into a deterministic, replayable artifact instead of a
one-off screen capture. It is used to:

- Record CLI demos and documentation screengrabs as committed `.cast` files that render
  identically every time (no flaky video capture, no OS-specific fonts/cursors baked in).
- Capture a workflow or custom-command run as a proof-of-run artifact (CI evidence, runbook
  reproduction).
- Script fully synthetic "simulated" terminal sessions (typed commands with jitter, styled
  prompts) for docs and mindset demos, without actually running the commands shown.

## Key Commands

| Command | Purpose |
|---|---|
| `atmos cast play <input.cast>` | Play an asciicast recording back in the terminal |
| `atmos cast render <input.cast> --output=<path>` | Render a recording to GIF, MP4, HTML, ASCII, PNG, or JPEG |
| `atmos cast render <input.cast> --output=<path> --format=<fmt>` | Render with an explicit format when the output filename's extension doesn't indicate one |

### Global `--cast` Flag / `ATMOS_CAST`

Any Atmos command can be recorded directly, without going through `atmos cast`:

```shell
atmos terraform plan vpc -s dev --cast=demo.cast   # record raw asciicast
atmos terraform plan vpc -s dev --cast=demo.gif    # record, then auto-render to gif, discard the intermediate .cast
atmos terraform plan vpc -s dev --cast              # auto value: record without keeping/rendering (ad hoc)
ATMOS_CAST=demo.cast atmos terraform plan vpc -s dev
```

`--cast`/`ATMOS_CAST` accepts the same extensions as `atmos cast render`'s output-format
inference. When the value ends in a renderable extension (not `.cast`), Atmos records to a
temporary cast in the OS temp dir, renders it to the requested output, and removes the
intermediate `.cast`. When the value is `.cast`, the raw recording is kept at that path. `--cast`
with no path (or `true`/`1`/`yes`/`on` via env) records without an explicit target.

Recording can also be enabled automatically for every command via `atmos.yaml`:

```yaml
# atmos.yaml
cast:
  recording:
    enabled: true
    base_path: "./casts"   # directory intermediate/auto casts are written under
    input: false           # also record stdin keystrokes, not just output
    width: 120
    height: 30
```

Automatic (config-enabled) recording skips `--help` and shell-completion invocations so casual
`--help` calls aren't captured; an explicit `--cast`/`ATMOS_CAST` still records help output.

## Output Format Inference

Both `atmos cast render` and the `--cast`/`ATMOS_CAST` flag infer the render format from the
output file's extension. Supported extensions/formats: `.gif`, `.mp4`, `.html`, `.ascii`, `.png`,
`.jpg`, and `.jpeg` (`.jpg`/`.jpeg` both map to the `jpeg` renderer). `.cast` means "keep the raw
recording, do not render."

```shell
atmos cast render demo.cast --output=demo.gif                 # format inferred from .gif
atmos cast render demo.cast --output=demo.html                # format inferred from .html
atmos cast render demo.cast --output=demo.out --format=html   # explicit --format for a non-matching extension
```

`--format` is required when the output path's extension doesn't map to a supported format (or has
none). If both the extension and `--format` imply a format, they must agree -- Atmos errors on a
conflict (e.g. `--output=demo.gif --format=html`).

## Workflow and Custom-Command Step Types

The same recording/rendering engine is available as native step types inside `workflows:` and
custom-command `steps:` -- no wrapper script needed.

### `type: cast`

A `cast` step wraps a set of child steps (or a scripted shell session) and records everything they
produce into an asciicast. It supports two `mode:` values:

**`mode: steps` (default)** -- runs nested child steps (any step type, including `type: simulate`)
through the normal step executor and records their combined output:

```yaml
steps:
  - type: cast
    name: demo-plan
    mode: steps           # optional, this is the default
    width: 120
    height: 30
    output:                # CastOutput: any subset of these paths
      cast: demo.cast       # keep the raw recording
      gif: demo.gif         # also render to gif
      html: demo.html       # also render to html
    steps:
      - type: simulate
        mode: typed
        text: "atmos terraform plan vpc -s dev"
      - type: atmos
        command: terraform plan vpc -s dev
```

**`mode: session`** -- drives an interactive shell directly with a scripted list of session
actions instead of running typed steps. Each child step's `type:` is the session action:

| Action | Fields | Purpose |
|---|---|---|
| `write` | `text`, `rate` | Type literal text into the session |
| `key` | `key`, `interval` | Send a single keypress (e.g. `enter`, `tab`, `ctrl+c`) |
| `pause` | `duration` | Wait a fixed duration before the next action |
| `wait` | `text` or `regex` (exactly one), `timeout` | Block until the session output matches |

```yaml
steps:
  - type: cast
    name: demo-session
    mode: session
    shell: bash
    steps:
      - type: write
        text: "atmos workflow deploy-vpc -s dev\n"
      - type: wait
        regex: "Apply complete!"
        timeout: 2m
      - type: key
        key: enter
```

Cast-level fields: `width`/`height` (recording dimensions), `rate` (default output pacing),
`title`, `command`, `env`, and `output` (a `CastOutput`: `cast`, `gif`, `mp4`, `html`, `ascii`,
`png`, `jpg` -- any subset). `defaults.cast` (`rate`/`width`/`height`) and `defaults.simulate`
(see below) set shared defaults for child steps so they don't need to repeat the same values.

### `type: simulate`

A `simulate` step (used as a child of a `mode: steps` cast) replays scripted terminal activity
without actually running a command -- useful for docs/demo casts that show a command's output
verbatim rather than depending on live infrastructure. Two `mode:` values:

- `mode: typed` (default) -- types `text` character-by-character at the terminal, then presses
  enter. Fields: `text` (required), `rate` (base per-character delay), `jitter` (0-1, randomizes
  per-character timing deterministically so re-recording the same script is reproducible),
  `duration` (delay before the simulated enter/output), `interval` (pause after the step, before
  the next), `cursor` (show the terminal cursor while typing), and `prompt` (`SimulatePrompt`:
  `text` and `style`, where `style` is one of the theme styles -- `body`, `command`, `label`,
  `muted`, `info`, `notice`).
- `mode: prompt` -- renders just the prompt (and optional cursor), without typing anything; used
  to show an idle prompt between recorded actions.

```yaml
steps:
  - type: cast
    mode: steps
    defaults:
      simulate:
        prompt:
          text: "$ "
          style: command
        jitter: 0.3
    steps:
      - type: simulate
        mode: typed
        text: "atmos version"
        cursor: true
      - type: atmos
        command: version
      - type: simulate
        mode: prompt
```

## Common Patterns

- **Committed docs screengrabs**: record demo commands to a `.cast` file, commit it, and render
  it at doc-build time (or check in the rendered HTML/image too). The `.cast` source stays
  deterministic across environments; re-rendering never depends on terminal fonts or timing.
- **CI proof-of-run artifacts**: wrap a workflow's real steps in `type: cast` with `output.gif` or
  `output.html` so a pipeline can publish what actually ran, not just its exit code.
- **Fully synthetic demos**: use `type: simulate` steps exclusively (no real `atmos`/`shell`
  children) to script a "fake" but visually authentic terminal walkthrough for marketing/docs
  content, independent of any live stack or component.
- **Mixing real and simulated steps**: interleave `type: simulate` (to narrate/type the command)
  with the real `type: atmos`/`type: shell` step that actually executes it, so the recording
  looks hand-typed while the executed output is genuine.
- **Synthetic directory transitions**: when the story enters a generated or nested directory,
  show `type: simulate` with `cd <directory>` before the first command there. This is
  display-only; keep `working_directory` on every real child step so execution does not depend
  on simulated terminal state.
- Prefer `atmos cast render` over ad hoc `--cast=<gif>` one-liners when a `.cast` source should be
  kept and re-rendered into multiple formats later; use `--cast=<ext>` for quick one-shot capture
  when only the rendered artifact matters.
