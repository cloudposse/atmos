import assert from 'node:assert/strict';
import test from 'node:test';

import {applyIdleSkip} from './playback.mjs';

test('applyIdleSkip compresses an initial prompt-to-output gap', () => {
  const events = [
    [0, 'o', '> '],
    [0.1, 'o', 'a'],
    [0.2, 'o', 't'],
    [0.3, 'o', 'm'],
    [0.4, 'o', 'o'],
    [0.5, 'o', 's'],
    [0.6, 'o', '\n'],
    [5.2, 'o', 'Stacks\n'],
    [5.4, 'o', 'dev\n'],
  ];

  const skipped = applyIdleSkip(events, 1.5);

  assert.ok(Math.abs(skipped[7][0] - 2.1) < 0.000001);
  assert.ok(Math.abs(skipped[8][0] - 2.3) < 0.000001);
  assert.deepEqual(skipped.map((event) => event[2]), events.map((event) => event[2]));
});
