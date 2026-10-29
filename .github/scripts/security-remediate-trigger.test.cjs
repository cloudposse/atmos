const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { test } = require('node:test');

// This is a Bash hook for local Unix agent sessions, not a Windows action.
const options = { skip: process.platform === 'win32' };
const payload = {
  tool_name: 'Bash',
  tool_input: { command: 'git push' },
  tool_response: { stderr: 'GitHub found 2 vulnerabilities on the default branch' },
};

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'security-hook-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const hooks = path.join(root, '.claude', 'hooks');
  const bin = path.join(root, 'bin');
  fs.mkdirSync(hooks, { recursive: true });
  fs.mkdirSync(bin);
  const hook = path.join(hooks, 'security-remediate-trigger.sh');
  fs.copyFileSync(path.join(__dirname, '../../.claude/hooks/security-remediate-trigger.sh'), hook);
  const mock = (name, source) => fs.writeFileSync(path.join(bin, name), `#!/usr/bin/env node\n${source}`, { mode: 0o755 });
  mock('git', 'process.stdout.write("test-branch\\n");');
  mock('gh', `
    const args = process.argv.slice(2);
    if (args[0] === 'repo') {
      process.stdout.write('cloudposse/atmos\\n');
    } else {
      require('node:assert/strict').ok(args.includes('GET') && args.includes('state=open') && args.includes('--paginate'));
      const dependabot = args[1].endsWith('/dependabot/alerts');
      process.stdout.write(dependabot ? process.env.TEST_DB : process.env.TEST_SCANNERS);
      if (process.env.TEST_FAIL === (dependabot ? 'dependabot' : 'scanning')) process.exit(1);
    }
  `);
  return (env = {}, input = payload) => {
    const result = spawnSync('bash', [hook], {
      encoding: 'utf8', input: JSON.stringify(input), timeout: 30000,
      env: { ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH}`,
        TEST_DB: 'db:1\ndb:2\n',
        TEST_SCANNERS: '{"number":1,"tool":"Semgrep OSS"}\n{"number":2,"tool":"CodeQL"}\n{"number":3,"tool":"Semgrep OSS"}\n',
        TEST_FAIL: '', ...env },
    });
    assert.equal(result.status, 0, result.stderr || String(result.error));
    return result.stdout;
  };
}

test('reports repository backlog with scanner counts and preserves alert-set deduplication', options, t => {
  const run = fixture(t);
  const output = JSON.parse(run()).hookSpecificOutput.additionalContext;
  assert.match(output, /repository-wide security backlog/);
  assert.match(output, /5 open alerts \(Dependabot: 2; code scanning: 3 \(CodeQL: 1, Semgrep OSS: 2\)\)/);
  assert.match(output, /not a count of vulnerabilities introduced by this push/);
  assert.match(output, /atmos lint --changed/);
  assert.equal(run(), '');
  assert.match(run({ TEST_DB: 'db:1\ndb:2\ndb:3\n' }), /6 open alerts/);
});

test('handles either empty source without losing the other source', options, t => {
  assert.match(fixture(t)({ TEST_SCANNERS: '' }), /2 open alerts \(Dependabot: 2; code scanning: 0\)/);
  assert.match(fixture(t)({ TEST_DB: '' }), /3 open alerts \(Dependabot: 0; code scanning: 3/);
  assert.equal(fixture(t)({ TEST_DB: '', TEST_SCANNERS: '' }), '');
});

test('failed API queries never emit partial totals or mark the alert set as seen', options, t => {
  for (const source of ['dependabot', 'scanning']) {
    const run = fixture(t);
    assert.equal(run({ TEST_FAIL: source }), '');
    assert.match(run(), /5 open alerts/);
  }
});

test('ignores dry runs, non-push commands, and pushes without the vulnerability notice', options, t => {
  const run = fixture(t);
  for (const command of ['git push --dry-run', 'git push --tags', 'git status']) {
    assert.equal(run({}, { ...payload, tool_input: { command } }), '');
  }
  assert.equal(run({}, { ...payload, tool_response: {} }), '');
});
