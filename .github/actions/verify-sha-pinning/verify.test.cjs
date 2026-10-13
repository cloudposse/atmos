const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const retryModule = require('./retry.cjs');

// Execute the actual embedded verifier, so its allowlist/error handling cannot
// diverge from a duplicate implementation in a test.
const source = fs.readFileSync(path.join(__dirname, 'action.yml'), 'utf8');
const marker = '        script: |\n';
assert.ok(source.includes(marker));
const lines = source.slice(source.indexOf(marker) + marker.length).split('\n');
const scriptLines = [];
for (const line of lines) {
  if (line.trim() && !line.startsWith('          ')) break;
  scriptLines.push(line.slice(10));
}
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const verify = new AsyncFunction('require', 'github', 'core', 'process', scriptLines.join('\n'));
const sha = 'a'.repeat(40);

async function runVerifier(t, responses) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'verify-sha-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const workflowDir = path.join(dir, 'workflows');
  fs.mkdirSync(workflowDir);
  fs.writeFileSync(path.join(workflowDir, 'test.yml'), `steps:\n  - uses: owner/action@${sha} # v1\n  - uses: owner/action@${sha} # v1\n`);
  const allowlistFile = path.join(dir, 'allowlist.json');
  fs.writeFileSync(allowlistFile, JSON.stringify([
    { action: 'owner/action', description: 'Reviewed access block', references: ['https://example.com/review'] },
  ]));
  const outputs = {};
  const delays = [];
  let failure;
  let calls = 0;
  const core = { info() {}, warning() {}, error() {},
    setOutput(name, value) { outputs[name] = value; }, setFailed(message) { failure = message; } };
  const github = { rest: { git: { async getRef() {
    const response = responses[Math.min(calls++, responses.length - 1)];
    if (response instanceof Error) throw response;
    return response;
  } } } };
  const localRequire = name => name.endsWith('retry.cjs') ? {
    ...retryModule,
    createRetry: options => retryModule.createRetry({ ...options, now: () => 100000, sleep: async ms => delays.push(ms) }),
  } : require(name);
  await verify(localRequire, github, core, { env: {
    RUNNER_TEMP: dir, WORKFLOW_DIR: workflowDir, ALLOWLIST_FILE: allowlistFile, VERIFY_ACTION_PATH: __dirname,
  } });
  return { outputs, failure, calls, delays };
}

function blocked(headers) {
  return Object.assign(new Error('API request rejected'), { status: 403, response: { headers } });
}

test('actual verifier recovers from quota exhaustion and caches the verified tag', async t => {
  const result = await runVerifier(t, [blocked({ 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': '110' }),
    { data: { object: { type: 'commit', sha } } }]);
  assert.equal(result.outputs.status, 'pass');
  assert.equal(result.outputs.verified_count, 2);
  assert.equal(result.outputs.allowlisted_count, 0);
  assert.equal(result.calls, 2);
  assert.deepEqual(result.delays, [11000]);
  assert.equal(result.failure, undefined);
});

test('actual verifier never allowlists exhausted primary or secondary limits', async t => {
  for (const headers of [
    { 'x-ratelimit-remaining': '0', 'x-ratelimit-reset': '110' },
    { 'x-ratelimit-remaining': '42', 'retry-after': '1' },
  ]) {
    const result = await runVerifier(t, [blocked(headers)]);
    assert.equal(result.outputs.status, 'fail');
    assert.equal(result.outputs.failed_count, 2);
    assert.equal(result.outputs.allowlisted_count, 0);
    assert.equal(result.calls, 3);
    assert.match(result.failure, /2 action reference\(s\) failed/);
  }
});

test('actual verifier retains reviewed access-block handling', async t => {
  const result = await runVerifier(t, [blocked({ 'x-ratelimit-remaining': '42' })]);
  assert.equal(result.outputs.status, 'pass');
  assert.equal(result.outputs.allowlisted_count, 2);
  assert.deepEqual(result.delays, []);
});

test('actual verifier still rejects a SHA mismatch after quota recovery', async t => {
  const result = await runVerifier(t, [blocked({ 'retry-after': '1' }),
    { data: { object: { type: 'commit', sha: 'b'.repeat(40) } } }]);
  assert.equal(result.outputs.status, 'fail');
  assert.equal(result.outputs.failed_count, 2);
  assert.equal(result.outputs.allowlisted_count, 0);
  assert.equal(result.calls, 2);
  assert.match(result.failure, /SHA mismatch/);
});
