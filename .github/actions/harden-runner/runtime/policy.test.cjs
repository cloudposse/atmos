const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { readPolicy, prepare } = require('./policy.cjs');

test('loads only the selected policy and removes duplicate destinations', () => {
  assert.equal(readPolicy(JSON.stringify({a:['github.com:443','*.example.com:443','github.com:443'], b:['other.example:80']}), 'a'),
    'github.com:443 *.example.com:443');
});
for (const [label, policy, name] of [
  ['missing', {}, 'missing'], ['inherited', {}, 'toString'], ['empty', {a:[]}, 'a'],
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
    readFileSync(file) { assert.match(file, /endpoints\.json$/); return '{"test":["github.com:443"]}'; },
    appendFileSync(file, value) { written = [file, value]; },
  };
  prepare({ files, env: { INPUT_POLICY:'test', GITHUB_ENV:'/tmp/test-env' } });
  assert.deepEqual(written, ['/tmp/test-env', 'ATMOS_HARDEN_RUNNER_ENDPOINTS=github.com:443\n']);
});
test('every packaged policy validates', () => {
  const content=fs.readFileSync(path.join(__dirname, '..', 'endpoints.json'), 'utf8');
  for (const name of Object.keys(JSON.parse(content))) assert.ok(readPolicy(content, name));
});
