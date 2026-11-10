# Fix: Keep long AWS SSO verification URLs out of the dialog box

**Date:** 2026-11-10

**Issue:** [#3665](https://github.com/cloudposse/atmos/issues/3665)

## Summary

`atmos auth login` no longer draws the AWS SSO verification URL inside the
"AWS SSO Authentication Required" box when the URL is longer than 80 columns.
The URL is delivered below the box, and a new `settings.terminal.links` setting
selects how: a short clickable hyperlink, a clipboard copy, or the raw URL.

## Context

AWS IAM Identity Center now returns a 521-character `signin.aws` verification
URL from `StartDeviceAuthorization`. The device-authorization dialog rendered
the URL on one line inside a rounded-border box with no width limit, so the box
grew to 527 columns. In a normal terminal every line of the box wrapped, and
text copied from it could include border characters.

The AWS browser webflow dialog already kept URLs over 80 columns out of its box.
The SSO device-authorization dialog was a separate copy of the dialog code that
never received that change.

## Changes

- The SSO dialog now keeps a URL in the box only when it fits in 80 columns,
  using the same limit as the webflow dialog. The limit and the delivery of
  longer URLs are shared by both dialogs instead of being duplicated.
- A URL that does not fit is written below the box by `ui.WriteLinkPrompt`,
  in the form `settings.terminal.links` selects. The `ATMOS_TERMINAL_LINKS`
  environment variable overrides the setting, and the global `--links` flag
  overrides both:
  - `auto` (default): an OSC 8 hyperlink labelled "Open AWS sign-in" when the
    terminal is known to support one, otherwise the raw URL.
  - `osc8`: always the hyperlink. This is the override for terminals that
    support hyperlinks but cannot be detected, such as inside a container.
  - `clipboard`: copy the URL to the system clipboard. When no native clipboard
    is reachable, request an OSC 52 copy and also print the raw URL.
  - `raw`: always the raw URL.
- Hyperlink support is detected from an allowlist of terminal-identifying
  environment variables, because terminals offer no query for it. Detection
  reports no support inside `tmux` or `screen`.
- Output that is not a terminal always gets the raw URL and never touches the
  clipboard.
- `atmos auth console` delivers its console sign-in URL through the same
  prompt when it does not open a browser (`--no-open`, CI, or a failed launch).
  `--print-only` still writes the bare URL to stdout.
- The dialog code moved from `sso.go` to `sso_ui.go`.

## Validation

- Reproduced first: a test rendering the dialog with a 521-character URL failed
  with a 527-column box line.
- Unit tests cover the box width, URL placement at the 80/81-column boundary,
  every delivery mode, the clipboard fallback, mode parsing, the detection
  allowlist, and the config and environment-variable binding.
- Rendered the real dialog in a pseudo-terminal as an unidentified terminal,
  as WezTerm, as WezTerm inside tmux, and with `osc8` and `raw` forced.
- Not verified against a live IAM Identity Center sign-in, and the hyperlink was
  not clicked in a real terminal emulator.
