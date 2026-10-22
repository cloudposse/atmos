const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { parse } = require('./yaml.cjs');
const { readPolicy, prepare } = require('./policy.cjs');

test('loads only the selected policy and removes duplicate destinations', () => {
  assert.equal(readPolicy(JSON.stringify({a:['github.com:443','*.example.com:443','github.com:443'], b:['other.example:80']}), 'a'),
    'github.com:443 *.example.com:443');
});
for (const [label, policy, name] of [
  ['missing', {}, 'missing'], ['inherited', {}, 'toString'],
  ['non-array', {a:'github.com:443'}, 'a'], ['newline', {a:['github.com:443\nEVIL=true']}, 'a'],
  ['scheme', {a:['https://github.com:443']}, 'a'], ['CIDR', {a:['140.82.112.0/20:443']}, 'a'],
  ['non-string', {a:[443]}, 'a'], ['zero port', {a:['github.com:0']}, 'a'],
  ['out-of-range port', {a:['github.com:65536']}, 'a'],
]) {
  test(`rejects ${label} policy`, () => assert.throws(() => readPolicy(JSON.stringify(policy), name)));
}
test('pre-hook exports a validated one-line value for the following pre-hook', () => {
  let written;
  const files = {
    readFileSync(file) { assert.match(file, /allowlist\.yaml$/); return '{"test":["github.com:443"]}'; },
    appendFileSync(file, value) { written = [file, value]; },
  };
  prepare({ files, env: { INPUT_POLICIES:'test', GITHUB_ENV:'/tmp/test-env' } });
  assert.deepEqual(written, ['/tmp/test-env', 'ATMOS_HARDEN_RUNNER_ENDPOINTS=github.com:443\n']);
});
test('every packaged policy validates', () => {
  const content=fs.readFileSync(path.join(__dirname, '..', '..', '..', 'allowlist.yaml'), 'utf8');
  const names = Object.keys(parse(content));
  assert.ok(names.length > 0);
  for (const os of ['Linux', 'Windows', 'macOS']) {
    for (const name of names) assert.equal(typeof readPolicy(content, name, os), 'string');
  }
});

test('explicit empty policy adds no endpoints beyond upstream implicit destinations', () => {
  assert.equal(readPolicy('{"no-network":[]}', 'no-network'), '');
});

test('composes YAML groups without granting unselected endpoints', () => {
  const yaml = `
# Groups are shared by jobs.
github:
  - github.com:443
go:
  - proxy.golang.org:443
  - github.com:443
production:
  - production.example.com:443
`;
  assert.equal(readPolicy(yaml, ' github\n go github '), 'github.com:443 proxy.golang.org:443');
  assert.throws(() => readPolicy(yaml, 'github typo'), /Missing or invalid/);
});
for (const [name, content, selection] of [
  ['duplicate group', 'a: []\na: []', 'a'],
  ['empty document', '', 'a'],
  ['scalar document', 'text', 'a'],
  ['array document', '[]', 'a'],
  ['missing selection', 'a: []', undefined],
  ['blank selection', 'a: []', '  '],
  ['object endpoint', 'a: [{host: github.com}]', 'a'],
  ['recursive alias', 'a: &a [*a]', 'a'],
]) {
  test(`rejects ${name}`, () => assert.throws(() => readPolicy(content, selection)));
}
test('all workflow group selections resolve and every group has a consumer', () => {
  const github = path.resolve(__dirname, '..', '..', '..');
  const content = fs.readFileSync(path.join(github, 'allowlist.yaml'), 'utf8');
  const used = new Set();
  let consumers = 0;
  for (const file of fs.readdirSync(path.join(github, 'workflows'))) {
    if (!/\.ya?ml$/.test(file)) continue;
    const workflow = parse(fs.readFileSync(path.join(github, 'workflows', file), 'utf8'));
    for (const job of Object.values(workflow.jobs || {})) {
      for (const step of job.steps || []) {
        if (!step.uses?.startsWith('$/.github/actions/harden-runner')) continue;
        const selection = step.with?.policies;
        for (const os of ['Linux', 'Windows', 'macOS']) {
          assert.doesNotThrow(() => readPolicy(content, selection, os), `${file}/${os}`);
        }
        for (const name of selection.trim().split(/\s+/)) used.add(name);
        consumers++;
      }
    }
  }
  assert.ok(consumers > 0);
  assert.deepEqual([...used].sort(), Object.keys(parse(content)).sort());
});

test('pre-hook loads policies from its downloaded repository without checkout or installed dependencies', t => {
  const os = require('node:os');
  const { execFileSync } = require('node:child_process');
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'atmos-policy-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const github = path.join(temp, 'downloaded-repo', '.github');
  const runtime = path.join(github, 'actions', 'harden-runner', 'runtime');
  const workspace = path.join(temp, 'empty-workspace');
  fs.mkdirSync(runtime, { recursive: true });
  fs.mkdirSync(workspace);
  for (const file of ['prepare.cjs', 'policy.cjs', 'yaml.cjs']) {
    fs.copyFileSync(path.join(__dirname, file), path.join(runtime, file));
  }
  fs.writeFileSync(path.join(github, 'allowlist.yaml'), 'github:\n  - github.com:443\ngo:\n  - proxy.golang.org:443\n');
  const envFile = path.join(temp, 'env');
  execFileSync(process.execPath, [path.join(runtime, 'prepare.cjs')], {
    cwd: workspace,
    env: { ...process.env, INPUT_POLICIES: 'github\ngo', GITHUB_ENV: envFile },
  });
  assert.equal(fs.readFileSync(envFile, 'utf8'), 'ATMOS_HARDEN_RUNNER_ENDPOINTS=github.com:443 proxy.golang.org:443\n');
});

const scopedPolicies = `
shared: [github.com:443]
windows:
  runner-os: Windows
  endpoints: [windows.example.com:443]
macos:
  runner-os: macOS
  endpoints: [macos.example.com:443]
`;
for (const [os, expected] of [
  ['Linux', 'github.com:443'],
  ['Windows', 'github.com:443 windows.example.com:443'],
  ['macOS', 'github.com:443 macos.example.com:443'],
]) {
  test(`scopes OS services for ${os} while retaining shared destinations`, () => {
    assert.equal(readPolicy(scopedPolicies, 'shared windows macos', os), expected);
  });
}
for (const os of [undefined, '', 'linux', 'FreeBSD']) {
  test(`rejects missing or unsupported runner OS: ${os}`, () => {
    assert.throws(() => readPolicy(scopedPolicies, 'windows', os), /Runner OS is required/);
  });
}
for (const group of [
  { 'runner-os': 'windows', endpoints: [] },
  { 'runner-os': 'Windows', endpoint: [] },
  { 'runner-os': 'Windows', endpoints: 'windows.example.com:443' },
  { 'runner-os': 'Windows', endpoints: ['https://windows.example.com:443'] },
  { 'runner-os': 'Windows', endpoints: ['windows.example.com:65536'] },
  null,
]) {
  test(`rejects malformed scoped group even on another OS: ${JSON.stringify(group)}`, () => {
    assert.throws(() => readPolicy(JSON.stringify({ a: group }), 'a', 'Linux'));
  });
}
test('pre-hook uses GitHub RUNNER_OS to omit other operating systems', () => {
  let value;
  prepare({
    files: { readFileSync: () => scopedPolicies, appendFileSync: (_, content) => { value = content; } },
    env: { INPUT_POLICIES: 'shared windows macos', RUNNER_OS: 'Linux', GITHUB_ENV: '/tmp/env' },
  });
  assert.equal(value, 'ATMOS_HARDEN_RUNNER_ENDPOINTS=github.com:443\n');
});
test('Linux build excludes OS background-service DNS, including the NXDOMAIN regression', () => {
  const github = path.resolve(__dirname, '..', '..', '..');
  const workflow = parse(fs.readFileSync(path.join(github, 'workflows', 'test.yml'), 'utf8'));
  const selection = workflow.jobs.build.steps.find(step => step.uses === '$/.github/actions/harden-runner').with.policies;
  const content = fs.readFileSync(path.join(github, 'allowlist.yaml'), 'utf8');
  const groups = parse(content);
  const linux = new Set(readPolicy(content, selection, 'Linux').split(' '));
  assert.ok(linux.has('github.com:443'));
  assert.ok(linux.has('proxy.golang.org:443'));
  for (const name of ['windows-services', 'macos-services']) {
    for (const endpoint of groups[name].endpoints) assert.ok(!linux.has(endpoint), endpoint);
  }
  for (const os of ['Linux', 'Windows', 'macOS']) {
    assert.ok(!readPolicy(content, selection, os).includes('pti.store.microsoft.com'));
  }
});
