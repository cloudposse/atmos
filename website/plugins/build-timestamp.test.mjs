import assert from 'node:assert/strict';
import test from 'node:test';

import buildTimestamp from './build-timestamp.js';

const { getBuildDate, getBuildTimestamp } = buildTimestamp;

function withSourceDateEpoch(value, callback) {
  const previous = process.env.SOURCE_DATE_EPOCH;
  if (value === undefined) {
    delete process.env.SOURCE_DATE_EPOCH;
  } else {
    process.env.SOURCE_DATE_EPOCH = value;
  }

  try {
    callback();
  } finally {
    if (previous === undefined) {
      delete process.env.SOURCE_DATE_EPOCH;
    } else {
      process.env.SOURCE_DATE_EPOCH = previous;
    }
  }
}

test('uses SOURCE_DATE_EPOCH as a reproducible UTC timestamp', () => {
  withSourceDateEpoch('1789689600', () => {
    assert.equal(getBuildTimestamp(), '2026-09-18T00:00:00.000Z');
    assert.equal(getBuildDate().getTime(), 1789689600000);
  });
});

test('falls back to the current time when SOURCE_DATE_EPOCH is unset', () => {
  withSourceDateEpoch(undefined, () => {
    const before = Date.now();
    const actual = getBuildDate().getTime();
    const after = Date.now();
    assert.ok(actual >= before && actual <= after);
  });
});

test('rejects malformed SOURCE_DATE_EPOCH values', () => {
  withSourceDateEpoch('not-a-timestamp', () => {
    assert.throws(() => getBuildDate(), /non-negative integer/);
  });
});

test('rejects SOURCE_DATE_EPOCH values outside the Date range', () => {
  withSourceDateEpoch('999999999999999999999999', () => {
    assert.throws(() => getBuildDate(), /supported date range/);
  });
});
