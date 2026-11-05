# VHS Directive Support Reference

Exhaustive directive-by-directive matrix for Atmos's built-in VHS-dialect interpreter
(`tape:`/`source:` on `type: cast`, and `atmos cast record`). This is a v1, subset-compatible
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
| `Set Width <n>` | step recording width (columns) | VHS **pixels**, converted to columns: `floor((Width - 2*Padding) / (FontSize*0.6 + LetterSpacing))` |
| `Set Height <n>` | step recording height (rows) | VHS **pixels**, converted to rows: `floor((Height - 2*Padding) / (FontSize*LineHeight))` |
| `Set FontSize`/`Padding`/`LineHeight`/`LetterSpacing` | inputs to the Width/Height conversion | Defaults 22 / 60 / 1.0 / 0; order-independent; they do not change rendering fonts |
| `Set TypingSpeed <dur>` | step write/typing rate | e.g. `Set TypingSpeed 35ms` |
| `Set WaitTimeout <dur>` | default timeout for translated `Wait` actions | applies to every `Wait`/`Wait+Screen`/`Wait+Line` that doesn't carry its own `@<duration>` |
| `Output <path>` | step `output:` (`CastOutput`), one field per format | extension-inferred exactly like `atmos cast render`; tape-selected rendered outputs overwrite existing files; hand-written rendered paths remain protected even in mixed output settings; see extension table below |
| `Require <program>` | same tool-presence check as `type: require` (`tools: [program]`) | fails fast, before anything runs, if not on `PATH` |
| `Source <file>` | inlines another tape file's directives at that point | resolved relative to the step's `working_directory` (or CWD) -- the same fixed base `source` itself resolves against, held constant across every nested `Source`, never rebased to the directory of whichever file a `Source` happens to be read from (matches real `vhs`'s single-invocation-CWD model) |

**Size conversion details.** With VHS's default 1200x600, the result is 81 columns by 21 rows.
Results below 20 columns or 5 rows are clamped with a warning (usually the value was written as
cells). The step's explicit `width:`/`height:` always win -- use them to size in cells.

## `@<duration>` Modifiers

| Form | Effect |
|---|---|
| `Type@50ms "..."` | per-character typing speed for that `Type` |
| `Backspace@100ms 5` (any keypress) | delay between the repeats |
| `Wait@10s /re/`, `Wait+Screen@10s /re/`, `Wait+Line@10s /re/` | per-`Wait` timeout, overriding `Set WaitTimeout` |

Under `mode: steps`, `Wait` and `Wait@...` are still dropped with a warning.

## Literal Text

- `{{ ... }}` inside a `Type` is typed and run as written; tape-derived steps never go through Go
  templating (hand-written steps still do).
- Double-quoted strings are verbatim, like VHS: no backslash escapes (`Type "printf 'a\nb'"` types
  exactly that). Use backticks or single quotes to embed double quotes.

## Wait Semantics (Session Mode)

A wait only matches output produced after the most recent input action (write or key), so the echo
of the command just typed and output from earlier commands cannot satisfy it. This holds for
hand-written `wait` actions too. A timeout fails the cast with
`timed out waiting for cast output: /re/ not seen within 10s`.

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
| `.webm` | **No -- hard error at tape-parse time** (the message names the extension and lists the supported ones). Fix: change the `Output` line to `.mp4` or another supported extension. Real tapes in `demo/landing/*.tape` target `.webm` (for the website's `atmos demo record` pipeline) -- this is a genuine gap you will hit interpreting any of them as-is. |
| `.txt` | **No -- hard error at tape-parse time.** Fix: use `.ascii` instead (same plain-text, no-ANSI-codes output). |

## Cosmetic -- Warns, Then Ignored

No cast equivalent exists for these; the interpreter logs a warning naming the key and value, then
continues without applying it, since the recording engine's own theme/rendering system already
produces a consistent, themed result:

- `Set FontFamily`
- `Set Theme`
- `Set Margin`
- `Set BorderRadius`
- `Set Framerate`
- `Set CursorBlink`
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
| Bare/standalone `Delete`, `Insert`, `Home`, `End` | Same |
| `Ctrl+Alt+<letter>`, `Alt+<char>`, `Shift+Tab`, `Shift+<letter>` | Same |
| `Ctrl+<letter>` (e.g. `Ctrl+C`) | Same |
| `Type "text"` with no trailing `Enter` | Sends a bare keystroke to an already-running interactive program (e.g. `Type "q"` to quit `less` mid-command) -- has no discrete-step equivalent |
| `Hide` ... `Show`, regardless of contents | There is no PTY pass over the tape under `mode: steps`, so the interpreter can't look inside a `Hide` block to tell setup-only content apart from anything else -- the `Hide`/`Show` token itself is the error, before its contents are ever inspected |

**Leading hidden setup under `mode: session`**: before visible activity, simple `export`, `cd`,
and `clear` commands can become step configuration instead of muted keystrokes. Values expand
like a shell (`export PATH=$PATH:/x`, quoted values, `cd ~`, bare `cd`). `unset` always replays
in the live shell, so it removes variables inherited from the process or enclosing workflow.
Compound commands, command substitution, and `cd -` also replay. Once replay begins, subsequent
commands in that block replay in order; later `Hide` blocks always replay literally.

**Setup under `mode: steps`**: `Hide`/`Show` remains an error. Simple visible `cd`, `export`,
`unset`, and `clear` commands update tracked setup for subsequent real steps. Tracked unsets
run before each subsequent shell command, so inherited variables are absent rather than empty.
Compound, substituted, and `cd -` commands execute normally with a warning that their state
changes cannot carry to later processes. Split these commands or use `mode: session`.
When hand-writing the equivalent YAML, `env:` cannot remove inherited variables: prepend
an explicit `unset` to each affected shell command, as shown in the skill's worked example.
Tape-derived steps recordings omit step-name labels unless the cast step sets `show.labels`.

**Note on `Screenshot`**: `Screenshot <path>` is explicitly *not* session-only -- it is supported
in both `mode: steps` and `mode: session`, and becomes a native `type: screenshot` child step. It
marks a point in the recording to render a still image from, which works identically whether the underlying recording came from a PTY session or a
sequence of real steps.

## Session Shell Startup

`mode: session` starts the shell with `shell_init: clean` by default (bash `--noprofile --norc`,
zsh `-f` plus `NO_PROMPT_SP`, fish `--no-config`, other shells `ENV=`; `BASH_SILENCE_DEPRECATION_WARNING=1`
always set). `user` loads normal startup files; any other value is an rc-file path. Precedence: step
`shell_init` > `atmos cast record --shell-init` > `cast.session.shell_init` > `clean`. Session mode
waits for the shell's first prompt before the first action.

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
