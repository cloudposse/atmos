# VHS Directive Support Reference

Exhaustive directive-by-directive matrix for Atmos's built-in VHS-dialect interpreter
(`tape:`/`tape_file:` on `type: cast`, and `atmos cast record`). This is a v1, subset-compatible
interpreter of the real [VHS](https://github.com/charmbracelet/vhs) tape DSL -- it parses and
executes tape text directly, in memory, on every run. It is not a wrapper around the `vhs`
binary and does not generate YAML on disk. See
[../SKILL.md](../SKILL.md) for the operational checklist and worked examples; this file is the
full reference only.

## Mapped to Real Cast Fields

An explicit step-level YAML field always wins over the value a tape directive would otherwise
imply -- these directives only fill in what the step's own YAML doesn't already set.

| Directive | Target | Notes |
|---|---|---|
| `Set Shell <name>` | step `shell:` | e.g. `Set Shell bash` |
| `Set Width <n>` | step recording width | pixel/column width, same field `atmos cast render` uses |
| `Set Height <n>` | step recording height | |
| `Set TypingSpeed <dur>` | step write/typing rate | e.g. `Set TypingSpeed 35ms` |
| `Set WaitTimeout <dur>` | default timeout for translated `Wait` actions | applies to every `Wait`/`Wait+Screen` line that doesn't specify its own timeout |
| `Output <path>` | step `output:` (`CastOutput`), one field per format | extension-inferred exactly like `atmos cast render`; see extension table below |
| `Require <program>` | same tool-presence check as `type: require` (`tools: [program]`) | fails fast, before anything runs, if not on `PATH` |
| `Source <file>` | inlines another tape file's directives at that point | resolved relative to the step's `working_directory` (or CWD) -- the same fixed base `tape_file` itself resolves against, held constant across every nested `Source`, never rebased to the directory of whichever file a `Source` happens to be read from (matches real `vhs`'s single-invocation-CWD model) |

## `Output` Extension Acceptance List

Identical to `atmos cast render`'s supported output formats:

| Extension | Supported |
|---|---|
| `.cast` | Yes -- keeps the raw asciicast recording |
| `.gif` | Yes |
| `.mp4` | Yes |
| `.html` | Yes |
| `.ascii` | Yes |
| `.png` | Yes |
| `.jpg` / `.jpeg` | Yes -- both map to the `jpeg` renderer |
| `.webm` | **No -- hard error at tape-parse time.** Fix: change the `Output` line to `.mp4` or another supported extension. Real tapes in `demo/landing/*.tape` target `.webm` (for the website's `atmos demo record` pipeline) -- this is a genuine gap you will hit interpreting any of them as-is. |
| `.txt` | **No -- hard error at tape-parse time.** Fix: use `.ascii` instead (same plain-text, no-ANSI-codes output). |

## Cosmetic, Silently Ignored

No cast equivalent exists for these; the interpreter accepts and ignores them without error or
warning, since the recording engine's own theme/rendering system already produces a consistent,
themed result:

- `Set FontFamily`
- `Set FontSize`
- `Set Theme`
- `Set Margin`
- `Set Padding`
- `Set BorderRadius`
- `Set Framerate`
- `Set CursorBlink`
- `Set LetterSpacing`
- `Set LineHeight`
- `Set MarginFill`
- `Set WindowBar`
- `Set PlaybackSpeed`
- `Set LoopOffset`
- `Set WaitPattern` -- out of scope for v1 specifically (not a general cosmetic no-op like the
  others: it exists in real VHS to set a *default* regex for bare `Wait` calls). Every real tape
  in this repo always repeats the pattern explicitly on each `Wait` line (`Wait /❯$/`), so this
  has not been a practical gap; a future version may map it onto the same default-timeout-style
  mechanism as `Set WaitTimeout`.

## Session-Only Directives

Legal **only** under `mode: session`. Attempting to interpret a tape containing any of these under
`mode: steps` is a hard parse-time error that names the specific offending line and instructs you
to switch to `mode: session`.

| Directive | Why session-only |
|---|---|
| Bare/standalone `Enter` | No discrete-step equivalent for "press this key" outside a `Type ... Enter` pair |
| Bare/standalone `Space` | Same |
| Bare/standalone `Tab` | Same |
| Bare/standalone `Up` | Same |
| Bare/standalone `Down` | Same |
| Bare/standalone `Left` | Same |
| Bare/standalone `Right` | Same |
| Bare/standalone `Backspace` | Same |
| Bare/standalone `Escape` | Same |
| Bare/standalone `PageUp` | Same |
| Bare/standalone `PageDown` | Same |
| `Ctrl+<letter>` (e.g. `Ctrl+C`) | Same |
| `Type "text"` with no trailing `Enter` | Sends a bare keystroke to an already-running interactive program (e.g. `Type "q"` to quit `less` mid-command) -- has no discrete-step equivalent |
| `Hide` ... `Show` around anything other than `export KEY=VALUE`, `unset VAR`, `cd <path>`, or `clear` | The common case (env/cwd setup, screen clearing) lifts to the step's native `env:`/`working_directory:` config and works under **both** modes -- see below. Anything beyond that (partial typing, mid-`Hide` `Set` changes, etc.) has no discrete-step equivalent |

**Exception that works under both modes**: a `Hide` ... `Show` block whose `Type ... Enter` lines
are *exactly* one of `export KEY=VALUE`, `unset VAR`, `cd <path>`, or `clear` is lifted to the
step's own `env:`/`working_directory:` fields instead of requiring `mode: session`. `Hide` in VHS
exists purely as a workaround for not having step-level env/cwd configuration -- Atmos already has
that natively, so this common case does not force session mode.

**Note on `Screenshot`**: `Screenshot <path>` is explicitly *not* session-only -- it is supported
in both `mode: steps` and `mode: session`. It marks a point in the recording to render a still
image from, which works identically whether the underlying recording came from a PTY session or a
sequence of real steps.

## Always a Hard Error (Both Modes)

These are never silently dropped -- a silently-dropped `Env`, for example, could produce a
subtly broken demo (a variable the rest of the tape depends on simply isn't set, and nothing
tells you why).

| Directive | Error | Rewrite |
|---|---|---|
| `Copy "text"` | Hard error | Replace with a direct `Type "text"` where the text is used |
| `Paste` | Hard error | Replace with a direct `Type "text"` at the paste point |
| `Env KEY value` | Hard error | Inline as a shell `export` line inside a `Hide`/`Show` block (or directly, if on camera): `Type "export KEY=value" Enter` |

Example rewrite:

```text
# Unsupported VHS directives
Env HOME "/tmp/demo"
Copy "some text to reuse"
Paste
```

```text
# Rewritten for the Atmos interpreter
Type `export HOME=/tmp/demo` Enter
Type "some text to reuse" Enter
```
