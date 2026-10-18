const assert = require('node:assert/strict');
const { test } = require('node:test');
const { createRetry, isRateLimitError } = require('./retry.cjs');

test('waits until the primary quota resets before resolving the tag again', async () => {
  const delays = [];
  const error = Object.assign(new Error('API rate limit exceeded for installation'), {
    status: 403,
    response: { headers: { 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': '110' } },
  });
  const retry = createRetry({ warning() {}, now: () => 100000, sleep: async ms => delays.push(ms) });
  let calls = 0;
  const value = await retry(async () => {
    if (++calls === 1) throw error;
    return 'verified-sha';
  }, 'Resolving owner/action@v1');
  assert.equal(value, 'verified-sha');
  assert.equal(calls, 2);
  assert.deepEqual(delays, [11000]);
});

function apiError(status, headers) {
  return Object.assign(new Error(`HTTP ${status}`), { status, response: { headers } });
}

function harness() {
  const delays = [];
  const warnings = [];
  const retry = createRetry({ warning: message => warnings.push(message), now: () => 100000,
    sleep: async ms => delays.push(ms) });
  return { retry, delays, warnings };
}

test('honors Retry-After and waits for the later primary reset when both exist', async () => {
  for (const [status, headers, expected] of [
    [403, { 'retry-after': '2' }, 2000],
    [429, { 'retry-after': '5' }, 5000],
    [403, { 'retry-after': '2', 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': '110' }, 11000],
    [403, { 'retry-after': '20', 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': '110' }, 20000],
    [403, { 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': '99' }, 1000],
  ]) {
    const { retry, delays } = harness();
    let calls = 0;
    assert.equal(await retry(async () => {
      if (++calls === 1) throw apiError(status, headers);
      return 'sha';
    }, 'tag'), 'sha');
    assert.deepEqual(delays, [expected]);
  }
});

test('backs off secondary limits without inventing a primary reset', async () => {
  const { retry, delays } = harness();
  let calls = 0;
  await retry(async () => {
    if (++calls < 3) throw apiError(429, {});
  }, 'tag');
  assert.deepEqual(delays, [60000, 120000]);
});

test('never retries access denials, missing refs, or invalid requests', async () => {
  for (const status of [400, 401, 403, 404, 422]) {
    const { retry, delays } = harness();
    const error = apiError(status, {});
    let calls = 0;
    await assert.rejects(retry(async () => { calls++; throw error; }, 'tag'), err => err === error);
    assert.equal(calls, 1);
    assert.deepEqual(delays, []);
  }
});

test('primary limits with missing or malformed reset data fail without more API calls', async () => {
  for (const reset of [undefined, '', 'tomorrow', '-1', 'Infinity']) {
    const { retry, delays } = harness();
    const error = apiError(403, { 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': reset });
    let calls = 0;
    const operation = async () => { calls++; throw error; };
    await assert.rejects(retry(operation, 'first tag'), err => err === error);
    await assert.rejects(retry(operation, 'next tag'), err => err === error);
    assert.equal(calls, 1);
    assert.deepEqual(delays, []);
  }
});

test('stops when the total rate-limit wait budget would exceed 65 minutes', async () => {
  const { retry, delays } = harness();
  const error = apiError(429, { 'retry-after': '2000' });
  let calls = 0;
  const operation = async () => { calls++; throw error; };
  await assert.rejects(retry(operation, 'tag'), err => err === error);
  await assert.rejects(retry(operation, 'another tag'), err => err === error);
  assert.equal(calls, 2);
  assert.deepEqual(delays, [2000000]);
});

test('shares the rate-limit wait budget across successful tag resolutions', async () => {
  const { retry, delays } = harness();
  const error = apiError(429, { 'retry-after': '2000' });
  let calls = 0;
  await retry(async () => { if (++calls === 1) throw error; }, 'first tag');
  await assert.rejects(retry(async () => { throw error; }, 'next tag'), err => err === error);
  assert.deepEqual(delays, [2000000]);
});

test('exhausted rate-limit retries fail closed and stop subsequent lookups', async () => {
  const { retry, delays } = harness();
  const error = apiError(429, { 'retry-after': '1' });
  let calls = 0;
  const operation = async () => { calls++; throw error; };
  await assert.rejects(retry(operation, 'tag'), err => err === error);
  await assert.rejects(retry(operation, 'next tag'), err => err === error);
  assert.equal(calls, 3);
  assert.deepEqual(delays, [1000, 1000]);
});

test('preserves bounded retries for transient server and transport errors', async () => {
  for (const error of [apiError(502, {}), new Error('socket hang up')]) {
    const { retry, delays, warnings } = harness();
    let calls = 0;
    await assert.rejects(retry(async () => { calls++; throw error; }, 'tag'), err => err === error);
    assert.equal(calls, 3);
    assert.deepEqual(delays, [250, 500]);
    assert.equal(warnings.length, 2);
    assert.equal(await retry(async () => 'sha', 'next tag'), 'sha');
  }
});

test('rate-limit classification preserves access-denial allowlist boundaries', () => {
  assert.equal(isRateLimitError(apiError(403, { 'x-ratelimit-remaining': '0' })), true);
  assert.equal(isRateLimitError(apiError(403, { 'retry-after': 'invalid' })), true);
  assert.equal(isRateLimitError(apiError(429, {})), true);
  assert.equal(isRateLimitError(apiError(403, { 'x-ratelimit-remaining': '42' })), false);
  assert.equal(isRateLimitError(apiError(403, {})), false);
  assert.equal(isRateLimitError(new Error('API rate limit exceeded')), false);
});

test('default clock and sleep can recover from a transient error', async () => {
  const retry = createRetry({ warning() {} });
  let calls = 0;
  assert.equal(await retry(async () => {
    if (++calls === 1) throw apiError(502, {});
    return 'sha';
  }, 'tag'), 'sha');
});
