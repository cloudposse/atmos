import assert from 'node:assert/strict';
import test from 'node:test';

import {applyIdleSkip, parseCast} from './playback.mjs';

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

test('parseCast keeps v2 absolute event times', () => {
  const {header, events} = parseCast([
    '{"version":2,"width":80,"height":24,"command":"echo hi"}',
    '[0.5,"o","hello"]',
    '[1.25,"e"," error"]',
  ].join('\n'));

  assert.equal(header.version, 2);
  assert.deepEqual(events, [
    [0.5, 'o', 'hello'],
    [1.25, 'e', ' error'],
  ]);
});

test('parseCast accumulates v3 relative event times and skips comments', () => {
  const {header, events} = parseCast([
    '{"version":3,"term":{"cols":80,"rows":24},"title":"Demo"}',
    '# comment',
    '[0.5,"o","hello"]',
    '[0.25,"x","0"]',
    '[0.75,"o"," world"]',
  ].join('\n'));

  assert.equal(header.version, 3);
  assert.deepEqual(events, [
    [0.5, 'o', 'hello'],
    [1.5, 'o', ' world'],
  ]);
});
