import assert from 'node:assert/strict';
import test from 'node:test';

import { performCopy } from './copy.mjs';

test('performCopy returns copied when the clipboard write succeeds', async () => {
  const result = await performCopy(async () => {}, 'atmos version');
  assert.equal(result, 'copied');
});

test('performCopy returns failed when the clipboard write rejects', async () => {
  const result = await performCopy(async () => {
    throw new Error('clipboard access denied');
  }, 'atmos version');
  assert.equal(result, 'failed');
});

test('performCopy passes the exact text through to the writer', async () => {
  let received;
  await performCopy(async (text) => {
    received = text;
  }, 'atmos version');
  assert.equal(received, 'atmos version');
});
