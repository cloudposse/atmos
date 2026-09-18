import test from 'node:test';
import assert from 'node:assert/strict';
import { pruneCandidates, prune } from './index.mjs';

const now = Date.parse('2026-09-17T12:00:00Z');
const day = 86400000;
const lineage = `go-cache-Linux-X64-go1.26.6-${'a'.repeat(64)}`;
const entry = (id, days, overrides = {}) => ({ id, key: `${lineage}-${id}`, ref: 'refs/heads/main',
  created_at: new Date(now - days * day).toISOString(), size_in_bytes: 100, ...overrides });

test('keeps newest two per exact lineage and protects recent generations and other caches', () => {
  const caches = [entry(1, 4), entry(2, 3), entry(3, 2), entry(4, .8), entry(5, .4), entry(6, .2),
    entry(7, 8, { ref: 'refs/pull/1/merge' }), entry(8, 8, { key: 'unrelated-8' }),
    entry(9, 8, { key: `${lineage.replace('Linux', 'Windows')}-9` }),
    entry(10, 8, { key: `${lineage.replace('a'.repeat(64), 'b'.repeat(64))}-10` }),
    entry(11, 8, { created_at: 'invalid' })];
  assert.deepEqual(pruneCandidates(caches, now).map(c => c.id), [3, 2, 1]);
  assert.equal(caches[0].id, 1);
});

test('recognizes all warmup cache families without combining them', () => {
  const caches = ['go-cache', 'race-go-cache', 'race-plan-go-cache', 'intel-go-cache'].flatMap((prefix, i) =>
    [1, 2, 3].map(n => entry(i * 10 + n, n, { key: `${lineage.replace('go-cache', prefix)}-${i * 10 + n}` })));
  assert.deepEqual(pruneCandidates(caches, now).map(c => c.id), [3, 13, 23, 33]);
});

test('inventories every page before deletion and tolerates an already evicted entry', async () => {
  const old = '2020-01-01T00:00:00Z';
  const caches = Array.from({ length: 101 }, (_, i) => entry(i + 1, 10, { created_at: old }));
  const calls = [];
  const request = async (url, options) => {
    calls.push([url, options.method ?? 'GET']);
    if (options.method === 'DELETE') return { ok: false, status: 404 };
    return { ok: true, json: async () => ({ actions_caches: url.includes('page=2&') ? caches.slice(100) : caches.slice(0, 100) }) };
  };
  assert.equal(await prune({ token: 'test', repository: 'owner/repo', dryRun: false, request }), 9900);
  assert.deepEqual(calls.slice(0, 3).map(c => c[1]), ['GET', 'GET', 'DELETE']);
  assert.equal(calls.length, 101);
});

test('dry run never deletes and failed inventory never proceeds to deletion', async () => {
  const request = async (_, options) => {
    assert.equal(options.method, undefined);
    return { ok: true, json: async () => ({ actions_caches: [1, 2, 3].map(n => entry(n, n, { created_at: `2020-01-0${n}T00:00:00Z` })) }) };
  };
  assert.equal(await prune({ token: 'test', repository: 'owner/repo', request }), 100);
  await assert.rejects(prune({ token: 'test', repository: 'owner/repo', request: async () => ({ ok: false, status: 403 }) }), /inventory failed/);
});
