---
title: 'Terminal Output That Adapts to Your Terminal, Your Pipe, and Your CI Job'
date: 2025-11-04T12:00:00.000Z
slug: zero-config-terminal-output
authors:
  - osterman
tags:
  - feature
  - dx
release: v1.198.0
---

You pipe a command into `jq` and get a parse error, because a progress message went to stdout along with the data.
You open a CI log and it is a wall of escape codes, because the tool decided it was talking to a terminal. Worse,
you scroll back through that same log and find an AWS secret key printed in full. Atmos output now adapts to
wherever it lands — colors degrade to what the terminal actually supports, data and messages go to separate
streams, and secrets are masked on the way out.

<!--truncate-->

## The Problem

Command-line output has to serve two readers who want opposite things. A human at a terminal wants color, icons,
and text wrapped to the window. A pipe, a redirect, or a CI runner wants plain bytes and nothing else. Tools that
guess wrong produce logs full of escape codes, or drop the formatting that made the output readable in the first
place.

Then there is the part that is not cosmetic. Infrastructure tooling handles AWS keys, API tokens, and sensitive
configuration values. Any of those can end up in command output, and from there into a CI log that is far more
widely readable than the credential ever was. Secrets leaking into logs was the primary driver for this work.

## The Fix

Atmos detects the environment once and adapts everything downstream. Nothing to configure, and the behavior is
the same on every command.

### Color degradation

- **TrueColor terminal** (iTerm2, Windows Terminal): full 24-bit colors
- **256-color terminal**: 256-color palette
- **16-color terminal** (basic xterm): ANSI colors
- **No color** (CI, `NO_COLOR=1`, pipes): plain text

### Width adaptation

- **Wide terminal** (120+ cols): uses full width with proper wrapping
- **Narrow terminal** (80 cols): wraps at 80 characters
- **Config override**: respects `atmos.yaml` `settings.terminal.max_width`
- **Unknown width**: sensible defaults

### TTY detection

- **Interactive terminal**: full styling, colors, icons, formatting
- **Piped** (`atmos deploy | tee`): plain text automatically
- **Redirected** (`atmos > file`): plain text automatically
- **CI environment**: detected, and interactivity disabled

### Markdown rendering

Formatted help and report output renders as styled markdown in a color terminal, as plain text formatting where
there is no color, and falls back to the raw content if rendering fails.

### Secret masking

Sensitive values are redacted before anything reaches a stream:

```json
{
  "aws_access_key_id": "AKIAIOSFODNN7EXAMPLE",
  "aws_secret_access_key": "***MASKED***"
}
```

No manual redaction needed. Atmos detects and masks:

- AWS access keys and secrets (AKIA\*, ASIA\*)
- Sensitive environment variable patterns
- Common token formats
- JSON/YAML quoted variants

### Channel separation

Command results go to stdout. Status messages go to stderr. That split is what makes the output safe to pipe:

```bash
atmos terraform output | jq .vpc_id
# Still sees progress on stderr:
# ℹ Loading configuration...
# ✓ Output retrieved!
```

The data reaches `jq` clean, and you still watch the run in your terminal.

## How to Use It

The defaults are the point, so most of the time you do nothing. When you need to override the detection — forcing
color for a screenshot, or stripping it for a log — Atmos respects the standard conventions.

### Environment variables

- `NO_COLOR=1` - disables all colors
- `CLICOLOR=0` - disables colors
- `FORCE_COLOR=1` - forces color even when piped
- `TERM=dumb` - uses plain text output
- `CI=true` - detects CI environment
- `ATMOS_FORCE_TTY=true` - forces TTY mode with sane defaults (for screenshots)
- `ATMOS_FORCE_COLOR=true` - forces TrueColor even for non-TTY (for screenshots)

### CLI flags

- `--no-color` - disables colors
- `--color` - enables color (only if TTY)
- `--force-color` - forces TrueColor even for non-TTY (for screenshots)
- `--force-tty` - forces TTY mode with sane defaults (for screenshots)
- `--redirect-stderr` - redirects UI to stdout

## Terminal Output Is Not Logging

Worth keeping straight, because they are configured separately and behave differently.

- **Terminal output**: user-facing messages, status updates, and command results
  - Goes to **stdout/stderr**
  - Formatted for humans
  - Respects TTY detection and color settings
  - Automatically masked for secrets

- **Logging**: system events, debugging, internal state
  - Goes to **log files** (or `/dev/stderr` if configured)
  - Machine-readable format
  - Controlled by `--logs-level`
  - Not affected by terminal capabilities

Read more in the [CLI Configuration](/cli/configuration) documentation (see `logs` section) and
[Global Flags](/cli/global-flags) for `--logs-level` and `--logs-file` options.

## Get Involved

If Atmos guesses wrong about your terminal, your pipeline, or your CI runner — or if anything sensitive makes it
through unmasked — that is worth reporting with the exact environment at [our issue tracker](https://github.com/cloudposse/atmos/issues), or raising in [Slack](https://slack.cloudposse.com/).
[Open an issue on GitHub](https://github.com/cloudposse/atmos/issues).
