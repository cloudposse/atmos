import test from 'node:test';
import assert from 'node:assert/strict';
import { GitHub } from './api.mjs';
import { checkShards } from './shards.mjs';

const options = { runId: '42', attempt: '2', target: 'macos', count: '2' };
const polling = { polls: 3, interval: 15, log() {} };
const job = (n, overrides = {}) => ({ name: `Acceptance Tests (macos, shard ${n}/2)`,
  status: 'completed', conclusion: 'success', ...overrides });

function fixture(listings = [[job(1), job(2)]]) {
  const waits = [];
  let index = 0;
  const api = {
    async run(id) { assert.equal(id, 42); return { run_attempt: 2 }; },
    async list(path, field) {
      assert.equal(path, '/actions/runs/42/jobs?filter=latest');
      assert.equal(field, 'jobs');
      const listing = listings[Math.min(index++, listings.length - 1)];
      if (listing instanceof Error) throw listing;
      return listing;
    },
    async sleep(ms) { waits.push(ms); },
  };
  return { api, waits };
}

test('accepts successful shards across partial reruns and ignores unrelated jobs', async () => {
  const { api, waits } = fixture([[job(1, { run_attempt: 1 }), job(2, { run_attempt: 2 }),
    { name: 'Acceptance Tests (macos)', status: 'in_progress' },
    { name: 'Acceptance Tests (windows, shard 1/2)', conclusion: 'failure' }]]);
  await checkShards(api, options, polling);
  assert.deepEqual(waits, []);
});

test('waits for eventually consistent conclusions, with a bounded timeout', async () => {
  const pending = [job(1), job(2, { status: 'in_progress', conclusion: null })];
  const eventual = fixture([pending, [job(1), job(2)]]);
  await checkShards(eventual.api, options, polling);
  assert.deepEqual(eventual.waits, [15]);
  const stuck = fixture([pending]);
  await assert.rejects(checkShards(stuck.api, options, polling), /never settled/);
  assert.deepEqual(stuck.waits, [15, 15]);
});

test('fails closed on missing, duplicate, extra, or incorrectly numbered shards', async () => {
  for (const jobs of [[job(1)], [job(1), job(1)], [job(1), job(2), job(3)],
    [job(1), job(3)], [job(1), job(2, { name: 'Acceptance Tests (macos, shard 2/3)' })]]) {
    const { api, waits } = fixture([jobs]);
    await assert.rejects(checkShards(api, options, polling), /Expected exactly/);
    assert.deepEqual(waits, []);
  }
});

test('every non-success conclusion fails the gate', async () => {
  for (const conclusion of ['failure', 'cancelled', 'skipped', 'timed_out', 'neutral', 'action_required']) {
    const { api } = fixture([[job(1), job(2, { conclusion })]]);
    await assert.rejects(checkShards(api, options, polling), /did not succeed/);
  }
});

test('rejects superseded attempts before listing and after a successful listing', async () => {
  for (const supersededAt of [1, 2]) {
    const { api } = fixture();
    let reads = 0;
    api.run = async () => ({ run_attempt: ++reads >= supersededAt ? 3 : 2 });
    await assert.rejects(checkShards(api, options, polling), /superseded/);
  }
});

test('retries transient API and transport errors but fails on definitive errors', async () => {
  for (const error of [Object.assign(new Error('unavailable'), { status: 502 }),
    Object.assign(new Error('rate limited'), { status: 429 }), new TypeError('fetch failed'),
    new DOMException('timeout', 'TimeoutError')]) {
    const { api, waits } = fixture([error, [job(1), job(2)]]);
    await checkShards(api, options, polling);
    assert.deepEqual(waits, [15]);
    const stuck = fixture([error]);
    await assert.rejects(checkShards(stuck.api, options, polling), e => e === error);
    assert.deepEqual(stuck.waits, [15, 15]);
  }
  for (const status of [401, 403, 404]) {
    const { api, waits } = fixture([Object.assign(new Error('denied'), { status })]);
    await assert.rejects(checkShards(api, options, polling), /denied/);
    assert.deepEqual(waits, []);
  }
});

test('rejects malformed inputs before contacting GitHub', async () => {
  for (const override of [{ target: 'macos-intel' }, { count: 0 }, { count: 1.5 },
    { runId: '' }, { attempt: 0 }]) {
    await assert.rejects(checkShards({}, { ...options, ...override }, polling), /Invalid/);
  }
});

test('paginates real API listings and reports HTTP failures with retry metadata', async () => {
  const pages = [];
  const api = new GitHub('owner/repo', 'test', async url => {
    if (!url.includes('/jobs?')) return { ok: true, json: async () => ({ run_attempt: 2 }) };
    const page = Number(new URL(url).searchParams.get('page'));
    pages.push(page);
    const jobs = page === 1 ? [job(1), ...Array.from({ length: 99 }, (_, i) => ({ name: `Other ${i}` }))] : [job(2)];
    return { ok: true, json: async () => ({ jobs }) };
  });
  await checkShards(api, options, polling);
  assert.deepEqual(pages, [1, 2]);
  const denied = new GitHub('owner/repo', 'test', async () => ({ ok: false, status: 403 }));
  await assert.rejects(checkShards(denied, options, polling), error => error.status === 403);
});
