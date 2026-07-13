import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { parseCast } from './playback.mjs';
import { parseEscape, replayTerminal } from './terminal.mjs';

const dirname = path.dirname(fileURLToPath(import.meta.url));

test('replayTerminal renders plain sequential lines', () => {
  assert.equal(replayTerminal('hello\nworld\n'), 'hello\nworld\n');
});

test('replayTerminal overwrites the current line on a bare carriage return', () => {
  const result = replayTerminal('Downloading 10%\rDownloading 50%\rDownloading 99%\n');
  assert.equal(result, 'Downloading 99%\n');
});

test('replayTerminal keeps SGR escapes embedded in the line for later rendering', () => {
  const result = replayTerminal('\x1b[31mred\x1b[0m\n');
  assert.equal(result, '\x1b[31mred\x1b[0m\n');
});

test('replayTerminal ESC[K clears an empty/fresh line', () => {
  const result = replayTerminal('\x1b[Khello\n');
  assert.equal(result, 'hello\n');
});

test('replayTerminal cursor show/hide embeds and removes the cursor marker', () => {
  const shown = replayTerminal('\x1b[?25hhi');
  assert.ok(shown.includes('hi'));
  const hidden = replayTerminal('\x1b[?25hhi\x1b[?25l');
  assert.equal(hidden, 'hi');
});

test('replayTerminal preserves unchanged lines after a cursor-up redraw', () => {
  // Matches the real-world Bubbletea/huh redraw pattern: move the cursor up
  // to the top of the previous frame, then walk back down re-emitting only
  // the lines that changed; unchanged lines are just a bare "\n".
  const frame1 = 'line0\nline1\nline2\nline3\nline4\n';
  const redraw = '\x1b[5A\n\nline2-updated\x1b[K\n\n\n';
  const result = replayTerminal(frame1 + redraw);
  assert.equal(result, 'line0\nline1\nline2-updated\nline3\nline4\n');
});

test('replayTerminal ESC[K after fresh content is a no-op (erase-to-end from the cursor, not the whole line)', () => {
  // The recorder observes ONLCR-translated "\r\n" for every redrawn line,
  // which normalizeTerminalText (index.tsx) collapses to a bare "\n" before
  // this ever runs — so a rewritten line's default "K" always lands right
  // after content already written this pass, and must not erase it.
  const frame1 = 'line0\nline1\n';
  const redraw = '\x1b[2Anew-line0\x1b[K\n\n';
  const result = replayTerminal(frame1 + redraw);
  assert.equal(result, 'new-line0\nline1\n');
});

test('replayTerminal ESC[2K stays destructive even after fresh content', () => {
  const frame1 = 'line0\n';
  const redraw = '\x1b[1Awritten\x1b[2K\n';
  const result = replayTerminal(frame1 + redraw);
  assert.equal(result, '\n');
});

test('replayTerminal cursor-up followed by ESC[K clear-then-draw still fully replaces the line', () => {
  const frame1 = 'line0\nline1\nline2\n';
  const redraw = '\x1b[3A\x1b[Kreplaced\n\n\n';
  const result = replayTerminal(frame1 + redraw);
  assert.equal(result, 'replaced\nline1\nline2\n');
});

test('replayTerminal cursor-down preserves an existing row it lands on', () => {
  const frame1 = 'line0\nline1\nline2\n';
  // Up 3 (back to line0), down 1 (lands on the existing "line1"), overwrite.
  const redraw = '\x1b[3A\x1b[1Bnew-line1\n';
  const result = replayTerminal(frame1 + redraw);
  const lines = result.split('\n');
  assert.equal(lines[0], 'line0');
  assert.equal(lines[1], 'new-line1');
  assert.equal(lines[2], 'line2');
});

test('replayTerminal cursor-down past unvisited rows only initializes the row it lands on', () => {
  const result = replayTerminal('top\n\x1b[2Bnew\n');
  const lines = result.split('\n');
  assert.equal(lines[0], 'top');
  assert.equal(lines[3], 'new');
});

test('parseEscape parses ESC[A / ESC[B with an implicit count of 1', () => {
  assert.deepEqual(parseEscape('\x1b[A', 0), {
    kind: 'cursorMove',
    sequence: '\x1b[A',
    nextIndex: 3,
    delta: -1,
  });
  assert.deepEqual(parseEscape('\x1b[B', 0), {
    kind: 'cursorMove',
    sequence: '\x1b[B',
    nextIndex: 3,
    delta: 1,
  });
});

test('parseEscape parses ESC[<n>A / ESC[<n>B with an explicit count', () => {
  assert.deepEqual(parseEscape('\x1b[19A', 0), {
    kind: 'cursorMove',
    sequence: '\x1b[19A',
    nextIndex: 5,
    delta: -19,
  });
  assert.deepEqual(parseEscape('\x1b[3B', 0), {
    kind: 'cursorMove',
    sequence: '\x1b[3B',
    nextIndex: 4,
    delta: 3,
  });
});

test('parseEscape still ignores sequences with no current handler (H, J, C, D)', () => {
  for (const sequence of ['\x1b[H', '\x1b[2J', '\x1b[5C', '\x1b[5D', '\x1b[?1049h']) {
    const parsed = parseEscape(sequence, 0);
    assert.equal(parsed.kind, 'ignore');
  }
});

test('regression: the recorded interactive-menu cast is not blank at the reported bug timestamp', () => {
  const castPath = path.join(
    dirname,
    '..',
    '..',
    '..',
    'static',
    'casts',
    'cli',
    'interactive-menu.cast',
  );
  const { events } = parseCast(readFileSync(castPath, 'utf8'));
  const normalized = events.map((event) => [event[0], event[1], event[2].replace(/\r\n/g, '\n')]);

  // The bug was reported at t=2.6s of a 12.6s recording: the entire visible
  // (auto-scrolled-to-bottom) viewport was blank.
  const upTo = normalized
    .filter((event) => event[0] <= 2.6)
    .map((event) => event[2])
    .join('');
  const rendered = replayTerminal(upTo);
  const tail = rendered.split('\n').slice(-19);
  const nonBlank = tail.filter((line) => stripAnsi(line).trim() !== '');
  assert.ok(
    nonBlank.length > 0,
    `expected at least one non-blank line in the visible tail at t=2.6s, got:\n${tail.join('\n')}`,
  );
});

function stripAnsi(input) {
  return input.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '');
}
