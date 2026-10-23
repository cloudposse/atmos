const { test } = require('node:test');
const assert = require('node:assert/strict');
const { resolverConfig, run } = require('./index.cjs');

const original = '# runner DNS\nsearch internal\nnameserver 127.0.0.100\nnameserver 127.0.0.53\nnameserver 168.63.129.16\n';
function fixture(options = {}) {
  const calls = [];
  const contents = new Map([
    ['/etc/resolv.conf', original],
    ['/home/agent/agent.json', JSON.stringify({ egress_policy: options.policy || 'block' })],
  ]);
  const env = { RUNNER_TEMP: '/tmp', GITHUB_STATE: '/tmp/state', ...options.env };
  const files = {
    readFileSync: file => contents.get(file),
    mkdtempSync: () => '/tmp/backup',
    writeFileSync: (file, value) => contents.set(file, value),
    appendFileSync: (file, value) => { calls.push(['state', value]); contents.set(file, value); },
  };
  const exec = (command, args) => {
    calls.push([command, ...args]);
    if (command === 'systemctl') return options.active || 'active';
    if (args[1] === 'ss') return options.listener === false ? '' : 'udp UNCONN 127.0.0.1:53 0.0.0.0:* users:(("agent",pid=123,fd=7))';
    if (args[1] === 'cp') {
      if (options.failCopy && args.includes('--remove-destination')) throw new Error('copy failed');
      contents.set(args.at(-1), contents.get(args.at(-2)));
    }
    return '';
  };
  return { calls, contents, dependencies: { files, exec, env, platform: 'linux', log() {} } };
}

test('preserves options, removes fallback nameservers, and is idempotent', () => {
  const repaired = resolverConfig(original);
  assert.match(repaired, /search internal/);
  assert.deepEqual(repaired.match(/^nameserver .+$/gm), ['nameserver 127.0.0.1']);
  assert.equal(resolverConfig(repaired), repaired);
  assert.throws(() => resolverConfig('nameserver 8.8.8.8\n'), /Unexpected/);
  assert.throws(() => resolverConfig(''), /Unexpected/);
});
test('backs up and registers recovery before replacing DNS', () => {
  const f = fixture(); run(f.dependencies);
  assert.equal(f.contents.get('/tmp/backup/original'), original);
  assert.equal(f.contents.get('/etc/resolv.conf'), resolverConfig(original));
  const registered = f.calls.findIndex(call => call[0] === 'state');
  const replaced = f.calls.findIndex(call => call.includes('--remove-destination'));
  assert.ok(registered > 0 && registered < replaced);
});
for (const [name, options] of [
  ['stopped agent', { active: 'inactive' }],
  ['transitional agent', { active: 'activating' }],
  ['audit policy', { policy: 'audit' }],
  ['missing listener', { listener: false }],
]) {
  test(`refuses ${name} without replacing DNS`, () => {
    const f = fixture(options);
    assert.throws(() => run(f.dependencies));
    assert.equal(f.contents.get('/etc/resolv.conf'), original);
    assert.ok(!f.calls.some(call => call.includes('--remove-destination')));
  });
}
test('rejects non-Linux runners', () => {
  const f = fixture(); assert.throws(() => run({ ...f.dependencies, platform: 'darwin' }), /Linux/);
});
test('retains recovery state when replacement fails', () => {
  const f = fixture({ failCopy: true });
  assert.throws(() => run(f.dependencies), /copy failed/);
  assert.match(f.contents.get('/tmp/state'), /dnsBackup=/);
});
test('cleanup retains repaired DNS while agent remains active', () => {
  const f = fixture({ env: { STATE_dnsBackup: '/tmp/backup' } });
  run({ ...f.dependencies, cleanup: true });
  assert.ok(!f.calls.some(call => call.includes('cp')));
});
test('cleanup restores original when agent has stopped', () => {
  const f = fixture({ active: 'inactive', env: { STATE_dnsBackup: '/tmp/backup' } });
  f.contents.set('/tmp/backup/original', original);
  f.contents.set('/etc/resolv.conf', resolverConfig(original));
  run({ ...f.dependencies, cleanup: true });
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
});
test('cleanup without saved state does not attempt repair', () => {
  const f = fixture(); run({ ...f.dependencies, cleanup: true });
  assert.deepEqual(f.calls, []);
});
test('explicitly disabled repair does not touch DNS or inspect the agent', () => {
  const f = fixture({ env: { 'INPUT_DNS-REPAIR': 'false' } });
  run(f.dependencies);
  assert.deepEqual(f.calls, []);
});
