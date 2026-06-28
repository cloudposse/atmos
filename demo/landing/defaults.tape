# Shared VHS settings for the landing-page demos.
#
# Every demo/landing/<section>.tape begins with `Source demo/landing/defaults.tape`.
# Keep this file settings-only: no `Output`, no typed commands, no actions — those
# belong in the per-section tapes so each renders an independent asset.
#
# Rendered from the repo root, e.g. `cd demo/landing-root && vhs demo/landing/hero.tape`
# (the `atmos demo record` custom command does this for you).

Require atmos

Set Shell bash
Set FontFamily "FiraCode Nerd Font"
# FontSize is tuned against Width: at 1280px wide, the terminal column count is
# roughly (Width - 2*Padding) / (0.6 * FontSize). FontSize 32 yields ~64 columns —
# big, legible text when the video is scaled into the landing-page card (24 was too
# small to read). (A larger FontSize means fewer columns and bigger on-screen text;
# don't go so high that the table-heavy demos wrap — ~64 cols keeps them intact.)
Set FontSize 32
Set TypingSpeed 35ms
# Custom theme "Atmos": a verbatim clone of VHS's built-in "Monokai Vivid" palette
# with ONLY the cursor changed from Monokai's red (#fb0007) to off-white (#f5f5f5).
# The red block cursor read like an error and clashed with the green ❯ prompt;
# off-white matches the terminal body text. VHS has no cursor-color command and no
# theme-file path, so a custom theme must be inlined here as JSON (this file is
# sourced by every demo/landing/<section>.tape, so the cursor changes everywhere).
Set Theme `{ "name": "Atmos", "black": "#121212", "red": "#fa2934", "green": "#98e123", "yellow": "#fff30a", "blue": "#0443ff", "magenta": "#f800f8", "cyan": "#01b6ed", "white": "#ffffff", "brightBlack": "#838383", "brightRed": "#f6669d", "brightGreen": "#b1e05f", "brightYellow": "#fff26d", "brightBlue": "#0443ff", "brightMagenta": "#f200f6", "brightCyan": "#51ceff", "brightWhite": "#ffffff", "background": "#121212", "foreground": "#f9f9f9", "selection": "#ffffff", "cursor": "#f5f5f5" }`
Set BorderRadius 0
Set Margin 0
Set Padding 24
Set Width 1280
Set Height 640
Set Framerate 30
Set CursorBlink false

# Each tape sets PS1 to a green "❯ " in a hidden block. VHS trims the prompt line to
# "❯" (no trailing space) when matching, so the pattern is /❯$/ — a bare `Wait` then
# blocks until a command finishes and the fresh prompt returns. This is the VHS
# "wait for output" capability that makes recording real, variable-duration commands
# possible (no brittle Sleep guesses). WaitTimeout caps a stuck Wait so a broken tape
# fails in a few minutes instead of hanging — use `atmos demo check <name>` (fast
# .ascii dry-run) to catch problems in seconds before a full render.
Set WaitTimeout 300s
Set WaitPattern /❯$/
