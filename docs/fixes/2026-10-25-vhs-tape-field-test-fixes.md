# Fix: VHS tape interpretation produced wrong recordings while exiting 0

**Date:** 2026-10-25

## Summary

A hands-on field test of VHS-dialect tape support (`tape:`/`tape_file:` on `type: cast` steps and
`atmos cast record <input.tape>`) found that ordinary VHS tapes recorded the wrong content while the
command still exited 0. This fix corrects tape translation, the session engine's `wait` matching,
session shell startup (now configurable with `shell_init`), and several `atmos cast record` flag and
rerun problems.

## Context

Findings from the field test, with what the user saw:

- `Set Width 1200` / `Set Height 600` (VHS pixels) became a 1200×600-cell terminal, producing an
  11579×13462 px GIF.
- `{{ ... }}` in typed text (for example `docker ps --format '{{.Names}}'`) was rendered as a Go
  template: steps mode recorded `<no value>`, workflows failed with `map has no entry for key`.
- Double-quoted tape strings had backslash escapes stripped (`printf 'a\nb'` typed as `printf 'anb'`);
  VHS reads them verbatim.
- Session `wait` matched the entire session history, including the echo of the command just typed, so
  `Wait /late/` after typing `sleep 3; echo late` returned instantly and the recording ended early.
  This predates the tape feature; the user approved fixing it for all session waits.
- Setup-command lifting used string prefixes: `cd sub && pwd` became the working directory
  `sub && pwd`, `export PATH=$PATH:/x` set a literal `$PATH`, `export MSG="hello world"` kept only
  `hello`, and `cd ~` became `<cwd>/~`.
- Session mode lifted a hidden `cd`/`export` into the initial environment even when it appeared after
  visible commands, so earlier commands already saw the later state.
- Session mode started the user's interactive shell with its rc files, leaking a personal prompt
  (email, git branch), zsh autosuggestions and the macOS bash deprecation banner into recordings, and
  the first keystroke raced the prompt. This also predates the tape feature; the user asked for it to
  be configurable because VHS offered no control.
- Rerunning a tape with an `Output` directive (or `atmos cast record --output=<rendered>`) failed with
  "render output already exists" only after the whole recording ran.
- `atmos cast record --output=x.gif --format=cast` wrote JSON into `x.gif` (and the reverse wrote a GIF
  into a `.cast` file); `--mode=bogus` was not rejected up front.
- Common VHS syntax was rejected: `Type@50ms`, `Wait+Screen@10s`, `Alt+B`, `Shift+Tab`, `Delete`,
  `Home`, `End`.
- Steps-mode tape recordings showed synthetic `[record_step_2]` / `✓ record_step_2 completed` labels;
  the rendered-output path printed "Cast recorded:" for a temp file that is deleted right after.

## Changes

- `pkg/runner/step/cast_tape_geometry.go`: converts VHS pixel `Width`/`Height` to cells using the VHS
  canvas inputs (FontSize, Padding, LineHeight, LetterSpacing with VHS defaults), clamps to 20×5 with a
  warning, and keeps explicit step `width`/`height` authoritative.
- `pkg/runner/step/cast_tape.go`, `cast_tape_session.go`, `cast_simulate.go`: tape-derived children are
  marked literal through the existing `LiteralFields` mechanism; simulate text honors it; Type/key/Wait
  `@` modifiers map to write `rate`, key `interval`, and wait `timeout`; tape `Output` targets set
  `CastOutput.Overwrite`; the unsupported-extension error names the extension and the supported list;
  tape-derived shell children use `show.labels: false`; hidden setup commands are lifted only in a
  leading Hide block.
- `pkg/runner/step/cast_tape_setup.go`: setup-command recognition uses `mvdan.cc/sh` parsing and
  expansion; only a single simple `cd`/`export`/`unset`/`clear` is lifted, anything compound runs as a
  normal command (with a steps-mode warning that its cwd/env change does not carry over).
- `pkg/asciicast/tape.go`, `tape_parser.go`: verbatim double-quoted strings; `@<duration>` modifiers;
  new keys (Delete, Insert, Home, End, `Alt+<char>`, `Shift+Tab`, `Shift+<letter>`, `Ctrl+Alt+<letter>`).
- `pkg/asciicast/session_actions.go`, `session.go`: waits match only output after the most recent input
  action; a timeout names the pattern and duration; the expected "signal: killed" from tearing down the
  shell is no longer joined onto the real error; the session waits for the shell's first prompt before
  the first action.
- `pkg/asciicast/session_shell.go`, `session_unix.go`, `session_windows.go`: `ShellInit` — `clean`
  (default: bash `--noprofile --norc`, zsh `-f`, fish `--no-config`, others `ENV=`), `user`, or an
  rc-file path; `BASH_SILENCE_DEPRECATION_WARNING=1` always set. New sentinel `ErrInvalidCastShellInit`.
- `pkg/asciicast/render.go`: `RenderOptions.Overwrite`.
- `pkg/asciicast/session_shell.go`, `session.go`: the `shell` setting is a command line (`shell: bash -o vi`,
  `atmos cast record --shell=...`); its arguments follow the `shell_init` arguments, and `shell_init: user` runs it
  as written. A shell that exits at startup fails with `ErrCastSessionShellExited`, its exit status, and its first
  output lines (so `sh --no-profile` reports `sh: --no-profile: invalid option` instead of a PTY write error). The
  styled default prompt gets bash `\[ \]` / zsh `%{ %}` zero-width markers so clean shells redraw typed lines
  correctly; other shells get the default prompt uncolored.
- `pkg/schema` (`WorkflowStep`/`Task` `shell_init`, `CastOutput.Overwrite`/`Intermediate` internal
  fields, `CastConfig.Session.ShellInit`), `pkg/config/load.go` (`ATMOS_CAST_SESSION_SHELL_INIT`),
  regenerated atmos.yaml schema, stack-manifest schema `shell_init`, `known_fields.go`.
  `shell_init` precedence: step field > `atmos cast record --shell-init` > atmos.yaml
  `cast.session.shell_init` > `clean`.
- `cmd/cast/record.go`: symmetric `--format`/extension conflict check, overwrite of the explicit
  `--output`, `--shell-init` flag (`ATMOS_CAST_RECORD_SHELL_INIT`), up-front `--mode`/`--format`
  validation, and no temp-path announcement for rendered targets.
- Docs and skills: `website/docs/steps/type/cast.mdx`, `website/docs/cli/commands/cast/record.mdx`, the
  cast configuration docs, `agent-skills/skills/atmos-vhs/`, and the VHS blog post.

## Validation

- `go build ./...` passed.
- `go test` passed for `./pkg/asciicast/...`, `./pkg/runner/step/...`, `./cmd/cast/...`,
  `./pkg/schema/...`, `./pkg/config/...`, `./pkg/datafetcher/...`, `./pkg/flags/...` and `./errors/...`,
  including `TestEmbeddedSchemaIsCurrent` after `go generate ./pkg/config/schema`. New regression tests
  cover each finding (pixel conversion and clamp warning, literal `{{ }}`, verbatim quotes, `@`
  modifiers, new keys, wait-after-input and its timeout message, shell-parser setup lifting, leading-Hide
  lifting, `shell_init` resolution via the test binary as the shell, render overwrite, and the `record`
  flag checks).
- A broader `go test ./cmd/...` run had one failure, `cmd/secret` `TestRunSecretKeygen_GeneratesKey`.
  It also fails on the unmodified branch HEAD (checked in a clean worktree) and is unrelated to this change.
- `./custom-gcl run --new-from-rev=origin/main` over the changed packages: 0 issues.
- End to end with a fresh `build/atmos`, every field-test repro was re-run:
  - `Set Width 1200`/`Set Height 600` produced an 81×21 cast and an 800×493 GIF.
  - `echo '{{.Names}}'`, `printf 'a\nb\n'`, `cd sub && pwd && echo CHAINED`,
    `export PATH=$PATH:/opt/extra`, `export MSG="hello world"` and `cd ~` all behaved as in a real
    shell, and steps-mode recordings no longer show step labels.
  - The Hide-ordering tape printed `FIRST=[]` and then `SECOND=[late]` in `sub`.
  - A stale-output Wait now times out with `/DONE/ not seen within ...` instead of passing silently.
  - With the default `clean` shell init, zsh showed the Atmos prompt with no personal prompt or
    autosuggestions, and the first keystroke landed after the prompt; `--shell-init=user` restored the
    user's prompt.
  - `--output=x.gif --format=cast`, `--output=y.cast --format=gif` and `--mode=bogus` fail up front;
    reruns to an existing rendered `--output` or tape `Output` succeed.
  - `Type@10ms`, `Wait@5s`, `Wait+Screen@6s`, `Backspace@50ms 3`, `Alt+B`, `Shift+Tab` and `Ctrl+C`
    parse and run.
- `--shell="bash -o vi"`, `--shell="sh --noprofile"`, `--shell=zsh`, and `--shell="bash --rcfile demo.rc"
  --shell-init=user` recorded correctly end to end with clean line redraws; `--shell="sh --no-profile"` failed
  with `cast session shell exited: "sh --no-profile" (exit status 2): sh: --no-profile: invalid option`.
- `website` build (`pnpm run build`) passed with no new broken links or MDX errors.
- Not done: regenerating the `atmos cast record --help` screengrab. In this headless environment the
  screengrab recorder rejects every cast ("forced color did not reach the recorded commands"), including
  unrelated commands, so the committed cast still shows the old flag list.

## Follow-ups

- Regenerate the `atmos cast record --help` screengrab (new `--shell-init` flag) with
  `gh workflow run screengrabs.yaml` or locally in a real terminal:
  `atmos --chdir=demo/casts casts generate screengrabs cli --filter="atmos cast record --help"`.
- Pre-existing on main, awaiting a decision: the static cast renderer (`pkg/asciicast/cellgrid.go`)
  drops backspace (`\b`) and erase sequences, so a tape's `Backspace` edits stay visible in ASCII/PNG
  output even though the shell received them.
