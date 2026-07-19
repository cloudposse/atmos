// A minimal terminal-state replayer for asciicast (.cast) playback: takes the
// raw concatenated output text up to some point in time and returns the
// rendered screen as a newline-joined string (with SGR escapes and a cursor
// placeholder still embedded, for CastPlayer's React renderer to style).
//
// This intentionally does not implement a full VT100/xterm emulator. It
// supports exactly the escape sequences that Atmos's own recordings emit:
// SGR styling, cursor show/hide, clear-line/display, and cursor-up/down (the
// redraw mechanism Bubbletea and huh use for in-place full-screen updates —
// move the cursor up N lines, then walk back down re-emitting only the lines
// that changed). Anything else (horizontal cursor movement, absolute
// positioning, alternate-screen-buffer) is not emitted by any cast this repo
// currently records, so it's left in the "ignore" bucket rather than adding
// untested column/grid-addressing handling.

export const CURSOR_MARKER = "";

export function replayTerminal(input) {
  const lines = [""];
  let row = 0;
  let carriageReturnPending = false;
  let cursorVisible = false;
  // True until something is written to `row` since the cursor last moved
  // onto it vertically (bare `\n`, or the new cursor-up/down handling). A
  // redrawn line always starts fresh — real terminals reach that state via
  // an explicit `\r`, but Atmos's recordings rely on the PTY's ONLCR
  // translation (`\n` -> `\r\n`) for that, and normalizeTerminalText (in
  // index.tsx, applied before this ever runs) collapses `\r\n` back down to
  // a bare `\n`, discarding the `\r` before it gets here. Without this flag,
  // a rewritten line landing on a row an earlier frame already wrote to
  // would get its new content appended after the old content instead of
  // replacing it, once cursor-up/down lets that row be revisited at all.
  let freshRow = true;

  const currentLine = () => lines[row] ?? "";
  const setCurrentLine = (line) => {
    lines[row] = line;
  };
  const removeCursor = () => {
    setCurrentLine(currentLine().split(CURSOR_MARKER).join(""));
  };
  const syncCursor = () => {
    removeCursor();
    if (cursorVisible) {
      setCurrentLine(currentLine() + CURSOR_MARKER);
    }
  };
  const append = (text) => {
    if (carriageReturnPending) {
      clearLine();
      carriageReturnPending = false;
    } else if (freshRow) {
      clearLine();
    }
    freshRow = false;
    removeCursor();
    setCurrentLine(currentLine() + text);
    if (cursorVisible) {
      setCurrentLine(currentLine() + CURSOR_MARKER);
    }
  };
  const clearLine = () => {
    setCurrentLine("");
    if (cursorVisible) {
      setCurrentLine(CURSOR_MARKER);
    }
  };
  // Huh clears a completed form with CSI K followed by CSI J. K has already
  // erased the active row's suffix, so the line-level model only needs to
  // blank rows below the cursor for default erase-display (CSI J / CSI 0J).
  // Keeping the active row lets the following CR + CSI 2K redraw replace it
  // normally, while removing stale menu rows from the previous form frame.
  const clearDisplayAfterCursor = () => {
    removeCursor();
    for (let index = row + 1; index < lines.length; index += 1) {
      lines[index] = "";
    }
    syncCursor();
  };
  // Move the cursor `delta` rows (positive = down, negative = up). A row
  // that already exists keeps whatever content it has until something is
  // actually written to it (see `freshRow` above) — that's what lets a
  // redraw's cursor-up + selective-line-rewrite pattern preserve lines that
  // are skipped entirely, while still cleanly replacing ones that aren't.
  const moveCursorVertically = (delta) => {
    removeCursor();
    row = Math.max(0, row + delta);
    if (lines[row] === undefined) {
      lines[row] = "";
    }
    freshRow = true;
    syncCursor();
  };

  for (let index = 0; index < input.length; ) {
    const char = input[index];

    if (char === "\x1b") {
      const parsed = parseEscape(input, index);
      if (!parsed) {
        index += 1;
        continue;
      }
      if (parsed.kind === "sgr") {
        append(parsed.sequence);
      } else if (parsed.kind === "clearLine") {
        // Erase-to-end-of-line (the default/bare "K" form, which is all any
        // current recording uses) only erases what's *after* the cursor.
        // Content this pass already wrote sits before it, so a default-K
        // that lands after real content is a genuine no-op — clearing here
        // would destroy what was just written. Explicit non-default params
        // (1/2, "erase whole line") stay unconditionally destructive, and a
        // default-K on a still-fresh row (nothing written yet) still clears,
        // covering "clear-then-draw" recordings that lead with K.
        const param = parsed.sequence.slice(2, -1);
        const isDefault = param === "" || param === "0";
        if (!isDefault || freshRow) {
          clearLine();
        }
      } else if (parsed.kind === "clearDisplay") {
        clearDisplayAfterCursor();
      } else if (parsed.kind === "cursorShow") {
        cursorVisible = true;
        syncCursor();
      } else if (parsed.kind === "cursorHide") {
        cursorVisible = false;
        removeCursor();
      } else if (parsed.kind === "cursorMove") {
        carriageReturnPending = false;
        moveCursorVertically(parsed.delta);
      }
      index = parsed.nextIndex;
      continue;
    }

    if (char === "\r") {
      carriageReturnPending = true;
      index += 1;
      continue;
    }

    if (char === "\n") {
      carriageReturnPending = false;
      moveCursorVertically(1);
      index += 1;
      continue;
    }

    if (char === "\b") {
      removeCursor();
      setCurrentLine(removeLastVisibleChar(currentLine()));
      syncCursor();
      index += 1;
      continue;
    }

    append(char);
    index += 1;
  }

  return lines.join("\n");
}

export function parseEscape(input, start) {
  const next = input[start + 1];
  if (next === "]") {
    const bel = input.indexOf("\x07", start + 2);
    const st = input.indexOf("\x1b\\", start + 2);
    const end = bel === -1 ? st : st === -1 ? bel : Math.min(bel, st);
    if (end === -1) {
      return null;
    }
    return {
      kind: "ignore",
      sequence: input.slice(start, end + (end === st ? 2 : 1)),
      nextIndex: end + (end === st ? 2 : 1),
    };
  }

  if (next !== "[") {
    return {
      kind: "ignore",
      sequence: input.slice(start, start + 2),
      nextIndex: start + 2,
    };
  }

  const match = /\x1b\[[0-?]*[ -/]*[@-~]/.exec(input.slice(start));
  if (!match || match.index !== 0) {
    return null;
  }

  const sequence = match[0];
  const final = sequence.at(-1);
  if (final === "m") {
    return { kind: "sgr", sequence, nextIndex: start + sequence.length };
  }
  if (sequence === "\x1b[?25h") {
    return { kind: "cursorShow", sequence, nextIndex: start + sequence.length };
  }
  if (sequence === "\x1b[?25l") {
    return { kind: "cursorHide", sequence, nextIndex: start + sequence.length };
  }
  if (final === "K") {
    return { kind: "clearLine", sequence, nextIndex: start + sequence.length };
  }
  if (final === "J") {
    const param = sequence.slice(2, -1);
    if (param === "" || param === "0") {
      return { kind: "clearDisplay", sequence, nextIndex: start + sequence.length };
    }
  }
  if (final === "A" || final === "B") {
    const param = sequence.slice(2, -1);
    const count = param === "" ? 1 : parseInt(param, 10) || 1;
    return {
      kind: "cursorMove",
      sequence,
      nextIndex: start + sequence.length,
      delta: final === "A" ? -count : count,
    };
  }
  return { kind: "ignore", sequence, nextIndex: start + sequence.length };
}

export function removeLastVisibleChar(input) {
  const match = /\x1b\[[0-?]*[ -/]*[@-~]$/.exec(input);
  if (match) {
    return input.slice(0, match.index);
  }
  return input.slice(0, -1);
}
