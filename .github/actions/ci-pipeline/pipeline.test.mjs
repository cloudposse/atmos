import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { GitHub, integer } from './api.mjs';
import { childRun, filename, parentIdentity, policy, resolveRoot, sourceIdentity, sourceTitle } from './source.mjs';
import { combined, collectReports, publishReports, report, resultForJobs } from './report.mjs';
import { coverageInputs, mergeCoverage } from './coverage.mjs';

const head = 'a'.repeat(40), revision = 'b'.repeat(40), base = 'c'.repeat(40);
const done = name => ({ name, status: 'completed', conclusion: 'success' });

function fixture() {
  const runs = [], jobs = new Map(), writes = [], checks = [];
  const pr = { number: 42, state: 'open', draft: false, head: { sha: head }, base: { repo: { full_name: 'cloudposse/atmos' } } };
  for (const [index, file] of ['ci-build-linux.yml', 'ci-build-macos.yml', 'ci-build-windows.yml', 'ci-test-go.yml'].entries()) {
    runs.push({ id: (index + 1) * 10, run_attempt: 1, path: `.github/workflows/${file}`, event: 'pull_request',
      head_sha: head, repository: { full_name: 'cloudposse/atmos' }, pull_requests: [{ number: 42 }],
      display_title: `CI revision ${revision} base ${base}`, status: 'completed', conclusion: 'success',
      created_at: '2026-09-17T10:00:00Z', html_url: `https://github.com/cloudposse/atmos/actions/runs/${(index + 1) * 10}` });
  }
  let id = 100;
  for (const [file, definition] of Object.entries(policy)) {
    const root = runs.find(run => filename(run) === definition.parent);
    const run = { ...root, id: id++, path: `.github/workflows/${file}`, event: 'workflow_run',
      head_sha: 'd'.repeat(40), display_title: sourceTitle(root.id, root.run_attempt), pull_requests: [] };
    runs.push(run);
    jobs.set(run.id, definition.jobs.map(done));
  }
  jobs.set(40, [done('[magefiles] unit tests'), done('[race] shard')]);
  const api = {
    repo: 'cloudposse/atmos',
    async run(id) {
      const run = runs.find(run => run.id === Number(id));
      assert.ok(run, `missing run ${id}`);
      return structuredClone(run);
    },
    async jobs(run) { return structuredClone(jobs.get(run.id) ?? []); },
    async list(path) {
      if (path.includes('/check-runs')) return checks;
      if (path.endsWith('/pulls')) return [pr];
      const match = /^\/actions\/workflows\/([^/]+)\/runs\?/.exec(path);
      assert.ok(match, `unexpected list ${path}`);
      const query = new URL('https://example.test' + path).searchParams;
      return structuredClone(runs.filter(run => filename(run) === match[1] && run.event === query.get('event') &&
        (!query.has('head_sha') || run.head_sha === query.get('head_sha'))));
    },
    async request(method, path, body) {
      if (method !== 'GET') { writes.push({ method, path, body }); return {}; }
      if (path === `/commits/${revision}`) return { parents: [{ sha: base }, { sha: head }] };
      if (path === '/pulls/42') return structuredClone(pr);
      throw new Error(`Unexpected request ${path}`);
    },
  };
  return { api, runs, jobs, writes, checks, pr, root: runs[0], child: runs[4] };
}

test('PR tests resolve the original merge revision, not the workflow_run default branch', async () => {
  const { api, child } = fixture();
  const source = await resolveRoot(api, child);
  assert.equal(source.revision, revision);
  assert.equal(source.root.head_sha, head);
  assert.equal(source.active, true);
  assert.equal(source.pr.number, 42);
});

test('merge queue revisions remain synthetic merge queue commits', async () => {
  const { api, root } = fixture();
  root.event = 'merge_group'; root.head_sha = revision; root.pull_requests = [];
  const source = await resolveRoot(api, root);
  assert.equal(source.revision, revision);
  assert.equal(source.pr, undefined);
  assert.equal(source.active, true);
});

test('fork PRs resolve through the commit association when pull_requests is empty', async () => {
  const { api, root } = fixture(); root.pull_requests = [];
  assert.equal((await resolveRoot(api, root)).pr.number, 42);
});

test('a superseded PR cannot publish or rerun', async () => {
  const { api, child, pr, writes } = fixture(); pr.head.sha = 'e'.repeat(40);
  assert.equal((await report(api, child)).active, false);
  assert.deepEqual(writes, []);
});

test('an old child cannot consume a new build attempt', async () => {
  const { api, root, child } = fixture(); root.run_attempt = 2;
  assert.equal((await resolveRoot(api, child)).active, false);
});

test('a newer source run supersedes a run even on the same PR head', async () => {
  const { api, root, runs } = fixture(); runs.push({ ...root, id: 500 });
  assert.equal((await resolveRoot(api, root)).active, false);
});

test('a child may only consume its declared platform', async () => {
  const { api, child } = fixture(); child.display_title = sourceTitle(20, 1);
  assert.equal((await resolveRoot(api, child)).active, false);
});

test('foreign repositories and forged merge revisions are rejected', async () => {
  const { api, root } = fixture(); root.repository.full_name = 'attacker/repo';
  await assert.rejects(resolveRoot(api, root), /repository mismatch/);
  root.repository.full_name = api.repo;
  api.request = async () => ({ parents: [{ sha: 'e'.repeat(40) }] });
  await assert.rejects(resolveRoot(api, root), /wrong head/);
});

test('source identities reject malformed IDs, titles, paths and events', () => {
  const { root, child } = fixture();
  for (const value of ['0', '-1', '1e99', '1; echo injected']) assert.throws(() => integer(value));
  assert.throws(() => parentIdentity({ ...child, display_title: 'CI source 1 attempt 1\nextra' }));
  assert.throws(() => sourceIdentity({ ...root, event: 'pull_request_target' }));
  assert.throws(() => sourceIdentity({ ...root, path: 'unknown.yml' }));
  assert.throws(() => sourceIdentity({ ...root, display_title: 'CI revision main base main' }));
});

test('child discovery excludes prior build attempts and chooses the latest delivery', async () => {
  const { api, root, child, runs } = fixture();
  runs.push({ ...child, id: 600, display_title: sourceTitle(root.id, 2) });
  runs.push({ ...child, id: 500 });
  assert.equal((await childRun(api, filename(child), root)).id, 500);
});

for (const conclusion of ['failure', 'skipped', 'cancelled', null]) {
  test(`a ${conclusion} shard never passes the aggregate`, () => {
    assert.equal(resultForJobs({ status: 'completed' }, [{ ...done('shard'), conclusion }], ['shard']), 'failure');
  });
}

test('missing and duplicate jobs fail closed, while an unfinished run stays pending', () => {
  assert.equal(resultForJobs({ status: 'completed' }, [], ['shard']), 'failure');
  assert.equal(resultForJobs({ status: 'completed' }, [done('shard'), done('shard')], ['shard']), 'failure');
  assert.equal(resultForJobs({ status: 'in_progress' }, [done('shard')], ['shard']), 'in_progress');
  assert.equal(resultForJobs({ status: 'completed' }, [done('shard')], ['shard']), 'success');
});

test('k3s waits for both platforms and propagates missing fixture failures', async () => {
  const { api, root, runs, jobs } = fixture();
  const source = await resolveRoot(api, root);
  const mac = runs.find(run => filename(run) === 'ci-test-k3s-macos.yml');
  mac.status = 'in_progress';
  let reports = await collectReports(api, source);
  assert.equal(reports.find(check => check.name === '[k3s] demo-helmfile').result, 'in_progress');
  mac.status = 'completed'; jobs.set(mac.id, []);
  reports = await collectReports(api, source);
  assert.equal(reports.find(check => check.name === '[k3s] demo-helmfile').result, 'failure');
  assert.equal(combined(['success', 'success']), 'success');
});

test('failed builds fail dependent required checks without waiting for a child', async () => {
  const { api, root } = fixture(); root.conclusion = 'failure';
  const reports = await collectReports(api, await resolveRoot(api, root));
  assert.equal(reports.find(check => check.name === 'Acceptance Tests (linux)').result, 'failure');
});

test('one completion reconciles all platforms and writes only the source revision', async () => {
  const { api, child, writes } = fixture();
  await report(api, child);
  for (const platform of ['linux', 'macos', 'windows']) {
    assert.ok(writes.some(write => write.body.name === `Acceptance Tests (${platform})` && write.body.conclusion === 'success'));
  }
  assert.ok(writes.length > 20);
  assert.ok(writes.every(write => write.path === '/check-runs' && write.body.head_sha === revision));
});

test('check updates require our external ID and the GitHub Actions app', async () => {
  const { api, root, checks, writes } = fixture();
  const name = 'Acceptance Tests (linux)';
  checks.push({ id: 1, name, external_id: `atmos-ci:${name}`, app: { id: 123 } });
  const source = await resolveRoot(api, root);
  await publishReports(api, source, [{ name, result: 'success', url: root.html_url }]);
  assert.equal(writes[0].method, 'POST');
  checks.push({ id: 2, name, external_id: `atmos-ci:${name}`, app: { id: 15368 } });
  await publishReports(api, source, [{ name, result: 'in_progress', url: root.html_url }]);
  assert.equal(writes[1].path, '/check-runs/2');
  assert.equal(writes[1].body.conclusion, undefined);
});

test('a push arriving during aggregation prevents any check writes', async () => {
  const { api, child, pr, writes } = fixture();
  const jobs = api.jobs;
  api.jobs = async run => { const result = await jobs(run); pr.head.sha = 'f'.repeat(40); return result; };
  await report(api, child);
  assert.deepEqual(writes, []);
});

test('coverage waits for Mage and all Linux shards, but not race completion', async () => {
  const { api, root, runs, jobs } = fixture();
  const source = await resolveRoot(api, root);
  runs.find(run => run.id === 40).status = 'in_progress';
  assert.equal((await coverageInputs(api, source)).ready, true);
  jobs.set(40, []);
  assert.equal((await coverageInputs(api, source)).ready, false);
});

test('coverage refuses to mix source revisions or a missing shard', async () => {
  const { api, root, runs, jobs, child } = fixture();
  const source = await resolveRoot(api, root);
  jobs.set(child.id, jobs.get(child.id).filter(job => job.name !== 'Acceptance Tests (linux, shard 1/10)'));

  assert.equal((await coverageInputs(api, source)).ready, false);
  runs.find(run => run.id === 40).display_title = `CI revision ${head} base ${base}`;
  assert.equal((await coverageInputs(api, source)).ready, false);
});

test('coverage merge rejects missing data before invoking any executable', () => {
  const directory = mkdtempSync(join(tmpdir(), 'ci-coverage-test-'));
  try { assert.throws(() => mergeCoverage(directory), /ENOENT/); }
  finally { rmSync(directory, { recursive: true }); }
});

test('REST client paginates and retries transient GETs but never repeats writes', async () => {
  let calls = 0;
  const client = new GitHub('cloudposse/atmos', 'test', async url => {
    calls++;
    if (calls === 1) return new Response('', { status: 503 });
    const page = new URL(url).searchParams.get('page');
    return Response.json({ jobs: Array(page === '1' ? 100 : 1).fill({ id: 1 }) });
  }, async () => {});
  assert.equal((await client.list('/jobs', 'jobs')).length, 101);
  assert.equal(calls, 3);
  calls = 0;
  client.transport = async () => { calls++; return new Response('', { status: 503 }); };
  await assert.rejects(client.request('POST', '/check-runs', {}), /503/);
  assert.equal(calls, 1);
});

test('coverage merging processes real Go coverage data for every shard', async () => {
  const { mkdirSync, writeFileSync, readFileSync } = await import('node:fs');
  const { execFileSync } = await import('node:child_process');
  const directory = mkdtempSync(join(tmpdir(), 'ci-coverage-real-'));
  try {
    writeFileSync(join(directory, 'go.mod'), 'module example.test/coverage\n\ngo 1.24.0\n');
    writeFileSync(join(directory, 'main.go'), 'package main\nfunc main() { println("covered") }\n');
    const binary = join(directory, process.platform === 'win32' ? 'app.exe' : 'app');
    const env = { ...process.env, GOWORK: 'off', GOFLAGS: '', GOTOOLCHAIN: 'local', CGO_ENABLED: '0' };
    execFileSync('go', ['build', '-cover', '-covermode=count', '-o', binary, '.'], { cwd: directory, env });
    for (let i = 1; i <= 10; i++) {
      const shard = join(directory, `acceptance-coverage-linux-shard-${i}`);
      mkdirSync(shard);
      execFileSync(binary, [], { env: { ...env, GOCOVERDIR: shard }, stdio: 'ignore' });
    }
    mergeCoverage(directory);
    assert.match(readFileSync(join(directory, 'coverage.out'), 'utf8'), /example.test\/coverage\/main.go:.* 10/);
  } finally { rmSync(directory, { recursive: true }); }
});

test('completed job listings get bounded retries for eventual consistency', async () => {
  let calls = 0, sleeps = 0;
  const api = new GitHub('cloudposse/atmos', 'test', async () => {
    calls++;
    return Response.json({ jobs: calls === 1 ? [] : [done('shard')] });
  }, async () => { sleeps++; });
  assert.deepEqual(await api.jobs({ id: 1, status: 'completed' }, ['shard']), [done('shard')]);
  assert.equal(calls, 2);
  assert.equal(sleeps, 1);
});

test('manual dispatch uses main as its comparison base rather than comparing a commit to itself', async () => {
  const { api, root } = fixture();
  root.event = 'workflow_dispatch'; root.head_sha = revision;
  root.display_title = `CI revision ${revision} base ${revision}`;
  const request = api.request;
  api.request = async (method, path, body) => path === '/commits/main' ? { sha: base } : request(method, path, body);
  assert.equal((await resolveRoot(api, root)).base, base);
});

test('unchanged check results do not consume write API quota', async () => {
  const { api, root, checks, writes } = fixture();
  const name = 'Acceptance Tests (linux)';
  checks.push({ id: 1, name, external_id: `atmos-ci:${name}`, app: { id: 15368 },
    status: 'completed', conclusion: 'success', details_url: root.html_url });
  await publishReports(api, await resolveRoot(api, root), [{ name, result: 'success', url: root.html_url }]);
  assert.deepEqual(writes, []);
});

test('a sibling build retry during aggregation prevents stale green publication', async () => {
  const { api, child, runs, writes } = fixture();
  const jobs = api.jobs;
  api.jobs = async run => {
    const result = await jobs(run);
    if (filename(run) === 'ci-test-windows.yml') runs.find(candidate => candidate.id === 20).run_attempt++;
    return result;
  };
  await report(api, child);
  assert.deepEqual(writes, []);
});

test('every suite policy has unique jobs and accepts its complete successful matrix', () => {
  for (const definition of Object.values(policy)) {
    assert.equal(new Set(definition.jobs).size, definition.jobs.length, definition.name);
    assert.equal(resultForJobs({ status: 'completed' }, definition.jobs.map(done), definition.jobs), 'success');
  }
});
