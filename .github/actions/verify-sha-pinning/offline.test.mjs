import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const action = fs.readFileSync(new URL('./action.yml', import.meta.url), 'utf8');
const scripts = action.split('        script: |\n').slice(1).map(section =>
  section.split('\n    - name:')[0].split('\n').map(line => line.replace(/^          /, '')).join('\n'));
assert.equal(scripts.length, 2, "Expected verifier and comment scripts in action.yml");
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;

function fixture(t, lines, allowlist = []) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'sha-pin-test-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const workflows = path.join(root, 'workflows');
  fs.mkdirSync(workflows);
  fs.writeFileSync(path.join(workflows, 'test.yml'), lines.join('\n'));
  const allowlistFile = path.join(root, 'allowlist.json');
  fs.writeFileSync(allowlistFile, JSON.stringify(allowlist));
  const outputs = {};
  const failures = [];
  const core = {
    info() {}, warning() {}, error() {},
    setOutput: (key, value) => { outputs[key] = value; },
    setFailed: message => failures.push(message),
  };
  const env = { VERIFY_ACTION_PATH: path.dirname(fileURLToPath(import.meta.url)), RUNNER_TEMP: root, WORKFLOW_DIR: workflows, ALLOWLIST_FILE: allowlistFile };
  return { env, outputs, failures, core };
}

async function verify(state, getRef, repos = {}, paginate) {
  await new AsyncFunction('require', 'process', 'github', 'core', scripts[0])(
    require, { env: state.env }, { rest: { git: { getRef }, repos }, paginate }, state.core);
  return JSON.parse(fs.readFileSync(state.outputs.results_file, 'utf8'));
}

const sha = 'a'.repeat(40);
const ref = repo => `      - uses: owner/${repo}@${sha} # v1.0.0`;
const quotaError = () => Object.assign(new Error('API rate limit exceeded: ' + 'x'.repeat(1000)), {
  status: 403, response: { headers: { 'x-ratelimit-remaining': '0' } },
});

test('quota exhaustion stops lookups, fails closed, and writes large results to a file', async t => {
  const state = fixture(t, Array.from({ length: 300 }, (_, i) => ref(`repo-${i}`)), [{
    action: 'owner/repo-0', description: 'Reviewed access restriction', references: ['https://example.com/review'],
  }]);
  let requests = 0;
  const results = await verify(state, async () => { requests++; throw quotaError(); });
  assert.equal(requests, 1);
  assert.equal(state.outputs.failed_count, 300);
  assert.equal(state.outputs.allowlisted_count, 0);
  assert.equal(state.outputs.status, 'fail');
  assert.equal(state.failures.length, 1);
  assert.equal(results.length, 300);
  assert.ok(fs.statSync(state.outputs.results_file).size > 131072);
  assert.equal(state.outputs.results_json, undefined);

  let comment;
  const github = { rest: { issues: {
    listComments: async () => ({ data: [] }),
    createComment: async args => { comment = args.body; },
  } } };
  const env = {
    RESULTS_FILE: state.outputs.results_file, VERIFIED_COUNT: '0', FAILED_COUNT: '300',
    UNPINNED_COUNT: '0', STATUS: 'fail',
  };
  await new AsyncFunction('require', 'process', 'github', 'core', 'context', scripts[1])(
    require, { env }, github, state.core,
    { issue: { number: 1 }, repo: { owner: 'owner', repo: 'repo' }, runId: 42 });
  assert.ok(comment.includes('Showing 20 of 300 references'));
  assert.ok(comment.includes('SHA Pin Verification Failed'));
  assert.ok(Buffer.byteLength(comment) < 65536);
});

test('cached successes remain verified after another lookup exhausts the quota', async t => {
  const state = fixture(t, [ref('cached'), ref('limited'), ref('cached'), ref('unresolved')]);
  let requests = 0;
  const results = await verify(state, async ({ repo }) => {
    requests++;
    if (repo === 'limited') throw quotaError();
    return { data: { object: { type: 'commit', sha } } };
  });
  assert.equal(requests, 2);
  assert.equal(state.outputs.verified_count, 2);
  assert.equal(state.outputs.failed_count, 2);
  assert.deepEqual(results.map(r => r.status), ['verified', 'error', 'verified', 'error']);
});

test('empty workflows write an empty result file', async t => {
  const state = fixture(t, ['name: no actions']);
  const results = await verify(state, async () => assert.fail('unexpected API request'));
  assert.deepEqual(results, []);
  assert.equal(state.outputs.status, 'pass');
});

test('ordinary lookup failures do not suppress verification of other repositories', async t => {
  const state = fixture(t, [ref('missing'), ref('valid')]);
  let requests = 0;
  await verify(state, async ({ repo }) => {
    requests++;
    if (repo === 'missing') throw Object.assign(new Error('Not Found'), { status: 404 });
    return { data: { object: { type: 'commit', sha } } };
  });
  assert.equal(requests, 2);
  assert.equal(state.outputs.verified_count, 1);
  assert.equal(state.outputs.failed_count, 1);
});

for (const quotaSource of ['getCommit', 'listTags']) {
  test(`quota exhaustion during ${quotaSource} stops all later uncached lookups`, async t => {
    const state = fixture(t, [ref('cached'), ref('mismatch'), ref('later'), ref('cached'), ref('mismatch')], [{
      action: 'owner/mismatch', description: 'Reviewed access restriction', references: ['https://example.com/review'],
    }]);
    const requests = [];
    const results = await verify(state, async ({ repo }) => {
      requests.push(`getRef:${repo}`);
      return { data: { object: { type: 'commit', sha: repo === 'mismatch' ? 'b'.repeat(40) : sha } } };
    }, {
      getCommit: async () => {
        requests.push('getCommit');
        if (quotaSource === 'getCommit') throw quotaError();
        return {};
      },
      listTags() {},
    }, async () => {
      requests.push('listTags');
      throw quotaError();
    });
    assert.deepEqual(requests, ['getRef:cached', 'getRef:mismatch', 'getCommit',
      ...(quotaSource === 'listTags' ? ['listTags'] : [])]);
    assert.deepEqual(results.map(r => r.status), ['verified', 'error', 'error', 'verified', 'error']);
    for (const result of results.filter(r => r.status === 'error')) {
      assert.match(result.detail, /API rate limit exceeded/);
    }
    assert.equal(state.outputs.verified_count, 2);
    assert.equal(state.outputs.failed_count, 3);
    assert.equal(state.outputs.allowlisted_count, 0);
    assert.equal(state.outputs.status, 'fail');
    assert.equal(state.failures.length, 1);
  });
}

for (const status of [403, 500]) {
  test(`comment lookup handles HTTP ${status} without losing its retry policy`, async t => {
    const state = fixture(t, []);
    const resultsFile = path.join(state.env.RUNNER_TEMP, 'results.json');
    fs.writeFileSync(resultsFile, '[]');
    const failure = Object.assign(new Error('lookup failed'), { status });
    let requests = 0;
    const github = { rest: { issues: { listComments: async () => {
      requests++;
      if (requests === 1) throw failure;
      return { data: [] };
    } } } };
    const run = () => new AsyncFunction('require', 'process', 'github', 'core', 'context', scripts[1])(
      require, { env: { RESULTS_FILE: resultsFile } }, github, state.core,
      { issue: { number: 1 }, repo: { owner: 'owner', repo: 'repo' }, runId: 42 });
    if (status === 403) {
      await assert.rejects(run, error => error === failure);
      assert.equal(requests, 1);
    } else {
      await run();
      assert.equal(requests, 2);
    }
  });
}
