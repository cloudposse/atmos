import assert from 'node:assert/strict';
import test from 'node:test';

import {
  MAX_WAIT_MS,
  NETWORK_ERROR_MESSAGE,
  POLL_INTERVAL_MS,
  SLOWDOWN_THRESHOLD_MS,
  SLOW_POLL_INTERVAL_MS,
  STILL_RENDERING_MESSAGE,
  createPoller,
  describeStatus,
  formatElapsed,
  interpretPollResponse,
  isSlow,
  nextPollIntervalMs,
} from './polling.mjs';

function headers(contentType) {
  return { get: (name) => (name.toLowerCase() === 'content-type' ? contentType : null) };
}

function jsonResponse(status, data) {
  return {
    status,
    headers: headers('application/json'),
    json: async () => data,
  };
}

// The legacy/cached shape: a ready artifact served as the file itself
// (non-JSON content type), which must be aborted without ever being read.
function legacyReadyResponse() {
  let jsonCalled = false;
  return {
    response: {
      status: 200,
      headers: headers('video/mp4'),
      json: async () => {
        jsonCalled = true;
        return {};
      },
    },
    get jsonCalled() {
      return jsonCalled;
    },
  };
}

function queueFetch(responses) {
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (responses.length === 0) throw new Error('fetch mock exhausted its queued responses');
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return next;
  };
  return { fetchImpl, calls };
}

// Advances fake timers and drains the microtask queue a few times so any
// chained `await fetch()` / `await response.json()` inside the poller's
// timer callback gets a chance to run before the next assertion.
async function tick(t, ms) {
  t.mock.timers.tick(ms);
  for (let i = 0; i < 10; i += 1) await Promise.resolve();
}

async function flush() {
  for (let i = 0; i < 10; i += 1) await Promise.resolve();
}

test('interpretPollResponse: 202 "queued" artifact status', () => {
  const outcome = interpretPollResponse(202, {
    data: { artifacts: [{ status: 'queued', progress: null }] },
  });
  assert.deepEqual(outcome, { kind: 'rendering', phase: 'queued', progress: null });
});

test('interpretPollResponse: 202 "processing" artifact status carries progress through', () => {
  const outcome = interpretPollResponse(202, {
    data: { artifacts: [{ status: 'processing', progress: { percent: 40, stage: 'encoding' } }] },
  });
  assert.deepEqual(outcome, {
    kind: 'rendering',
    phase: 'processing',
    progress: { percent: 40, stage: 'encoding' },
  });
});

test('interpretPollResponse: 200 with a "ready" artifact status is ready', () => {
  const outcome = interpretPollResponse(200, {
    data: { artifacts: [{ status: 'ready', url: 'https://example.test/token', progress: null }] },
  });
  assert.deepEqual(outcome, { kind: 'ready' });
});

test('interpretPollResponse: 200 with a "processing" artifact status is still rendering', () => {
  const outcome = interpretPollResponse(200, {
    data: { artifacts: [{ status: 'processing', progress: { percent: 40, stage: 'encoding' } }] },
  });
  assert.deepEqual(outcome, {
    kind: 'rendering',
    phase: 'processing',
    progress: { percent: 40, stage: 'encoding' },
  });
});

test('interpretPollResponse: 404 surfaces the not-found message verbatim', () => {
  const outcome = interpretPollResponse(404, {
    success: false,
    error: 'Cast file was not found at the requested commit and path',
  });
  assert.deepEqual(outcome, { kind: 'error', message: 'Cast file was not found at the requested commit and path' });
});

test('interpretPollResponse: 202 with a "failed" artifact is terminal', () => {
  const outcome = interpretPollResponse(202, {
    data: { artifacts: [{ status: 'failed', error: 'ffmpeg crashed' }] },
  });
  assert.deepEqual(outcome, { kind: 'error', message: 'ffmpeg crashed' });
});

test('interpretPollResponse: 500 reads the top-level error verbatim', () => {
  const outcome = interpretPollResponse(500, { success: false, error: 'render backend unavailable' });
  assert.deepEqual(outcome, { kind: 'error', message: 'render backend unavailable' });
});

test('interpretPollResponse: falls back to a generic message when the body has no error', () => {
  const outcome = interpretPollResponse(400, null);
  assert.deepEqual(outcome, { kind: 'error', message: 'Render failed (HTTP 400)' });
});

test('nextPollIntervalMs/isSlow switch at the 13-minute threshold', () => {
  assert.equal(isSlow(SLOWDOWN_THRESHOLD_MS - 1), false);
  assert.equal(nextPollIntervalMs(SLOWDOWN_THRESHOLD_MS - 1), POLL_INTERVAL_MS);
  assert.equal(isSlow(SLOWDOWN_THRESHOLD_MS), true);
  assert.equal(nextPollIntervalMs(SLOWDOWN_THRESHOLD_MS), SLOW_POLL_INTERVAL_MS);
});

test('formatElapsed renders m:ss', () => {
  assert.equal(formatElapsed(0), '0:00');
  assert.equal(formatElapsed(65_000), '1:05');
  assert.equal(formatElapsed(13 * 60 * 1000), '13:00');
});

test('describeStatus labels queued vs. processing and flags a slow render', () => {
  assert.equal(describeStatus({ status: 'checking' }), 'Checking…');
  assert.equal(
    describeStatus({ status: 'rendering', phase: 'queued', elapsedMs: 5_000, slow: false }),
    'Queued… 0:05',
  );
  assert.equal(
    describeStatus({ status: 'rendering', phase: 'processing', elapsedMs: 5_000, slow: false }),
    'Rendering… 0:05',
  );
  assert.equal(
    describeStatus({ status: 'rendering', phase: 'processing', elapsedMs: 800_000, slow: true }),
    'Rendering… 13:20 — taking longer than usual',
  );
});

test('createPoller: 202 -> 202 -> legacy non-JSON 200 downloads exactly once and polls every 3s', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const ready = legacyReadyResponse();
  const { fetchImpl, calls } = queueFetch([
    jsonResponse(202, { data: { artifacts: [{ status: 'processing', progress: null }] } }),
    jsonResponse(202, { data: { artifacts: [{ status: 'processing', progress: null }] } }),
    ready.response,
  ]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();
  assert.equal(updates.length, 1);
  assert.equal(updates[0].status, 'rendering');

  await tick(t, POLL_INTERVAL_MS);
  assert.equal(updates.length, 2);
  assert.equal(updates[1].status, 'rendering');

  await tick(t, POLL_INTERVAL_MS);
  assert.equal(updates.length, 3);
  assert.equal(updates[2].status, 'ready');

  assert.equal(calls.length, 3);
  assert.equal(ready.jsonCalled, false, 'the legacy non-JSON 200 response body must never be read');

  const readyUpdates = updates.filter((u) => u.status === 'ready');
  assert.equal(readyUpdates.length, 1, 'must reach the ready state exactly once');
});

test('createPoller: an immediate legacy non-JSON 200 is ready right away without reading the body', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const ready = legacyReadyResponse();
  const { fetchImpl, calls } = queueFetch([ready.response]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.gif', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();

  assert.equal(calls.length, 1);
  assert.deepEqual(updates, [{ status: 'ready', elapsedMs: 0 }]);
  assert.equal(ready.jsonCalled, false);
});

test('createPoller: an immediate 200 JSON body with status "ready" downloads right away', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl, calls } = queueFetch([
    jsonResponse(200, { data: { artifacts: [{ status: 'ready', url: 'https://example.test/token', progress: null }] } }),
  ]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();

  assert.equal(calls.length, 1);
  assert.deepEqual(updates, [{ status: 'ready', elapsedMs: 0 }]);
});

test('createPoller: a 200 JSON body with status "processing" keeps rendering and polls again', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl, calls } = queueFetch([
    jsonResponse(200, { data: { artifacts: [{ status: 'processing', progress: { percent: 40, stage: 'encoding' } }] } }),
    jsonResponse(200, { data: { artifacts: [{ status: 'ready', progress: null }] } }),
  ]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();
  assert.equal(updates.length, 1);
  assert.deepEqual(updates[0], {
    status: 'rendering',
    phase: 'processing',
    progress: { percent: 40, stage: 'encoding' },
    elapsedMs: 0,
    slow: false,
  });

  await tick(t, POLL_INTERVAL_MS);
  assert.equal(calls.length, 2, 'must poll again instead of getting stuck on the interim 200');
  assert.equal(updates.length, 2);
  assert.equal(updates[1].status, 'ready');
});

test('createPoller: a 404 surfaces the "not found" message inline instead of a network error', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl } = queueFetch([
    jsonResponse(404, { success: false, error: 'Cast file was not found at the requested commit and path' }),
  ]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();

  assert.deepEqual(updates, [
    { status: 'error', errorMessage: 'Cast file was not found at the requested commit and path', elapsedMs: 0 },
  ]);
});

test('createPoller: 500 surfaces the JSON error inline and never navigates', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl } = queueFetch([jsonResponse(500, { success: false, error: 'render backend unavailable' })]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.webm', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();

  assert.deepEqual(updates, [{ status: 'error', errorMessage: 'render backend unavailable', elapsedMs: 0 }]);
});

test('createPoller: a 202 seen past 13 minutes elapsed marks the render slow and polls at 10s', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl } = queueFetch([
    jsonResponse(202, { data: { artifacts: [{ status: 'processing', progress: null }] } }),
  ]);

  const updates = [];
  const poller = createPoller({
    url: 'https://example.test/cast.mp4',
    fetchImpl,
    onUpdate: (u) => updates.push(u),
    initialElapsedMs: SLOWDOWN_THRESHOLD_MS,
  });

  poller.start();
  await flush();

  assert.equal(updates.length, 1);
  assert.equal(updates[0].status, 'rendering');
  assert.equal(updates[0].slow, true);
});

test('createPoller: gives up at the 30-minute ceiling with a non-failure message, without polling again', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl, calls } = queueFetch([]);

  const updates = [];
  const poller = createPoller({
    url: 'https://example.test/cast.mp4',
    fetchImpl,
    onUpdate: (u) => updates.push(u),
    initialElapsedMs: MAX_WAIT_MS,
  });

  poller.start();
  await flush();

  assert.equal(calls.length, 0, 'must not fetch again once the ceiling is already reached');
  assert.deepEqual(updates, [{ status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS }]);
});

test('createPoller: a hung fetch is aborted at the remaining budget and reported as still-rendering', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const fetchImpl = (url, init) =>
    new Promise((_resolve, reject) => {
      init.signal.addEventListener('abort', () => reject(new Error('aborted')));
    });

  const updates = [];
  const poller = createPoller({
    url: 'https://example.test/cast.mp4',
    fetchImpl,
    onUpdate: (u) => updates.push(u),
    initialElapsedMs: MAX_WAIT_MS - 5_000,
  });

  poller.start();
  await flush();
  await tick(t, 5_000);

  assert.deepEqual(updates, [{ status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS }]);
});

test('createPoller: a slow round trip consumes real wall-clock time toward the ceiling, not just the nominal poll interval', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  // The first poll's fetch doesn't resolve for 29 minutes — nearly the
  // entire budget — before finally returning a "still rendering" 202. A
  // poller that tracked elapsed time as a nominal counter (polls-so-far *
  // interval) would see this as elapsedMs === 0 and hand the next poll a
  // fresh 30-minute budget; a wall-clock-based poller must instead see
  // elapsedMs ~= 29 minutes and enforce the ceiling shortly after, not
  // ~29 minutes later still.
  const slowDelayMs = 29 * 60 * 1000;
  let resolveFetch;
  // Real fetch rejects an in-flight request when its AbortSignal fires; the
  // mock must do the same so the second poll's own deadline (asserted below)
  // can actually cut it off instead of hanging forever.
  const fetchImpl = (url, init) =>
    new Promise((resolve, reject) => {
      resolveFetch = () =>
        resolve(jsonResponse(202, { data: { artifacts: [{ status: 'processing', progress: null }] } }));
      init.signal.addEventListener('abort', () => reject(new Error('aborted')));
    });

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();
  await tick(t, slowDelayMs);
  resolveFetch();
  await flush();

  assert.equal(updates.length, 1);
  assert.equal(updates[0].status, 'rendering');
  assert.equal(updates[0].elapsedMs, slowDelayMs, 'must report true wall-clock elapsed, not the nominal 0');

  // The reschedule (slowed to 10s past the 13-minute threshold) must respect
  // the real remaining budget (~1 minute) rather than restarting a fresh
  // 30-minute window from this poll's nominal elapsedMs.
  await tick(t, SLOW_POLL_INTERVAL_MS);
  await tick(t, MAX_WAIT_MS - slowDelayMs - SLOW_POLL_INTERVAL_MS);

  assert.equal(updates.length, 2);
  assert.deepEqual(updates[1], { status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS });
});

test('createPoller: a 202 response whose JSON body hangs forever is aborted at the deadline and reported as still-rendering', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const fetchImpl = (url, init) =>
    Promise.resolve({
      status: 202,
      headers: headers('application/json'),
      // Mirrors real fetch semantics: response.json()'s body read is tied to
      // the same AbortSignal as the request, so aborting the controller
      // after fetch() has already resolved still rejects a pending read.
      json: () =>
        new Promise((_resolve, reject) => {
          init.signal.addEventListener('abort', () => reject(new Error('aborted')));
        }),
    });

  const updates = [];
  const poller = createPoller({
    url: 'https://example.test/cast.mp4',
    fetchImpl,
    onUpdate: (u) => updates.push(u),
    initialElapsedMs: MAX_WAIT_MS - 5_000,
  });

  poller.start();
  await flush();
  await tick(t, 5_000);

  assert.deepEqual(updates, [{ status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS }]);
});

test('createPoller: a genuine network failure (not our own abort) is reported distinctly', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const fetchImpl = async () => {
    throw new TypeError('Failed to fetch');
  };

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();

  assert.deepEqual(updates, [{ status: 'error', errorMessage: NETWORK_ERROR_MESSAGE, elapsedMs: 0 }]);
});

test('createPoller: cancel() stops a pending poll from ever firing', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  const { fetchImpl, calls } = queueFetch([
    jsonResponse(202, { data: { artifacts: [{ status: 'processing', progress: null }] } }),
  ]);

  const updates = [];
  const poller = createPoller({ url: 'https://example.test/cast.mp4', fetchImpl, onUpdate: (u) => updates.push(u) });

  poller.start();
  await flush();
  assert.equal(updates.length, 1);

  poller.cancel();
  await tick(t, POLL_INTERVAL_MS);

  assert.equal(calls.length, 1, 'no further fetch after cancel()');
  assert.equal(updates.length, 1, 'no further updates after cancel()');
});
