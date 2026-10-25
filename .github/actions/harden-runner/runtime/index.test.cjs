const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { resolverConfig, run } = require('./index.cjs');

const original = '# runner DNS\nsearch internal\nnameserver 127.0.0.100\nnameserver 127.0.0.53\nnameserver 168.63.129.16\n';
function fixture(options = {}) {
  const calls = [];
  const logs = [];
  const contents = new Map([
    ['/etc/resolv.conf', original],
    ['/home/agent/agent.status', 'Initialized\n'],
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
    if (command === 'systemctl') {
      if (options.serviceError) throw options.serviceError;
      return options.active || 'active';
    }
    if (args[1] === 'ss') return options.listener === false ? '' : 'udp UNCONN 127.0.0.1:53 0.0.0.0:* users:(("agent",pid=123,fd=7))';
    if (args[1] === 'cp') {
      if (options.failCopy && args.includes('--remove-destination')) throw new Error('copy failed');
      contents.set(args.at(-1), contents.get(args.at(-2)));
    }
    return '';
  };
  return { calls, logs, contents, dependencies: { files, exec, env, platform: 'linux', log: message => logs.push(message) } };
}

test('preserves options, removes fallback nameservers, and is idempotent', () => {
  const repaired = resolverConfig(original);
  assert.match(repaired, /search internal/);
  assert.deepEqual(repaired.match(/^nameserver .+$/gm), ['nameserver 127.0.0.1']);
  assert.equal(resolverConfig(repaired), repaired);
  assert.throws(() => resolverConfig('nameserver 8.8.8.8\n'), /Unexpected/);
  assert.throws(() => resolverConfig(''), /Unexpected/);
});
test('backs up and registers recovery before replacing DNS', async () => {
  const f = fixture(); await run(f.dependencies);
  assert.equal(f.contents.get(path.join('/tmp/backup', 'original')), original);
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
  test(`refuses ${name} without replacing DNS`, async () => {
    const f = fixture(options);
    await assert.rejects(async () => run(f.dependencies));
    assert.equal(f.contents.get('/etc/resolv.conf'), original);
    assert.ok(!f.calls.some(call => call.includes('--remove-destination')));
  });
}
test('rejects non-Linux runners', async () => {
  const f = fixture(); await assert.rejects(async () => run({ ...f.dependencies, platform: 'darwin' }), /Linux/);
});
test('retains recovery state when replacement fails', async () => {
  const f = fixture({ failCopy: true });
  await assert.rejects(async () => run(f.dependencies), /copy failed/);
  assert.match(f.contents.get('/tmp/state'), /dnsBackup=/);
});
test('cleanup retains repaired DNS while agent remains active', async () => {
  const f = fixture({ env: { STATE_dnsBackup: '/tmp/backup' } });
  await run({ ...f.dependencies, cleanup: true });
  assert.ok(!f.calls.some(call => call.includes('cp')));
});
test('cleanup restores original when agent has stopped', async () => {
  const f = fixture({ active: 'inactive', env: { STATE_dnsBackup: '/tmp/backup' } });
  f.contents.set(path.join('/tmp/backup', 'original'), original);
  f.contents.set('/etc/resolv.conf', resolverConfig(original));
  await run({ ...f.dependencies, cleanup: true });
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
});
test('cleanup without saved state does not attempt repair', async () => {
  const f = fixture(); await run({ ...f.dependencies, cleanup: true });
  assert.deepEqual(f.calls, []);
});
test('explicitly disabled repair does not touch DNS or inspect the agent', async () => {
  const f = fixture({ env: { 'INPUT_DNS-REPAIR': 'false' } });
  await run(f.dependencies);
  assert.deepEqual(f.calls, []);
});

function readinessFixture() {
  const options = {};
  const f = fixture(options);
  let elapsed = 0;
  const sleeps = [];
  f.dependencies.now = () => elapsed;
  f.dependencies.sleep = async milliseconds => {
    sleeps.push(milliseconds);
    elapsed += milliseconds;
  };
  return { ...f, options, sleeps, elapsed: () => elapsed };
}

function assertNoDNSWrites(f) {
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
  assert.ok(!f.calls.some(call => call[0] === 'state' || call.includes('cp')));
  assert.ok(!f.contents.has('/tmp/backup/repaired'));
  assert.ok(!f.logs.some(line => line.includes('initialization completed')));
}

test('waits for completed Docker initialization before touching DNS', async () => {
  const f = readinessFixture();
  const read = f.dependencies.files.readFileSync;
  f.dependencies.files.readFileSync = file => {
    if (file === '/home/agent/agent.status' && f.elapsed() < 500) {
      assertNoDNSWrites(f);
      throw Object.assign(new Error('initialization is still running'), { code: 'ENOENT' });
    }
    return read(file);
  };
  await run(f.dependencies);
  assert.deepEqual(f.sleeps, [250, 250]);
  assert.equal(f.logs[0], 'StepSecurity initialization completed; proceeding with DNS verification.');
  assert.equal(f.contents.get('/etc/resolv.conf'), resolverConfig(original));
});

test('ready agent proceeds without polling', async () => {
  const f = readinessFixture();
  await run(f.dependencies);
  assert.deepEqual(f.sleeps, []);
});

test('empty status during creation is polled until initialized', async () => {
  const f = readinessFixture();
  f.contents.set('/home/agent/agent.status', '');
  const sleep = f.dependencies.sleep;
  f.dependencies.sleep = async ms => {
    await sleep(ms);
    f.contents.set('/home/agent/agent.status', 'Initialized\n');
  };
  await run(f.dependencies);
  assert.deepEqual(f.sleeps, [250]);
});

for (const state of ['missing', 'empty']) {
  test(`readiness deadline rejects ${state} status without DNS writes`, async () => {
    const f = readinessFixture();
    const read = f.dependencies.files.readFileSync;
    f.dependencies.files.readFileSync = file => {
      if (file !== '/home/agent/agent.status') return read(file);
      if (state === 'empty') return '';
      throw Object.assign(new Error('not ready'), { code: 'ENOENT' });
    };
    await assert.rejects(async () => run(f.dependencies), /initialization.*60000ms/);
    assert.equal(f.elapsed(), 60000);
    assertNoDNSWrites(f);
  });
}

for (const [name, change, expected] of [
  ['unexpected status', f => f.contents.set('/home/agent/agent.status', 'Failed'), /Unexpected agent initialization status/],
  ['status read error', f => {
    const read = f.dependencies.files.readFileSync;
    f.dependencies.files.readFileSync = file => {
      if (file === '/home/agent/agent.status') throw Object.assign(new Error('access denied'), { code: 'EACCES' });
      return read(file);
    };
  }, /access denied/],
  ['service stops while waiting', f => { f.options.active = 'failed'; }, /not active/],
  ['service inspection fails', f => { f.options.serviceError = new Error('systemctl unavailable'); }, /systemctl unavailable/],
  ['policy changes while waiting', f => f.contents.set('/home/agent/agent.json', '{"egress_policy":"audit"}'), /block mode/],
  ['policy becomes unreadable', f => f.contents.set('/home/agent/agent.json', 'invalid'), /JSON/],
]) {
  test(`readiness rejects ${name} without DNS writes`, async () => {
    const f = readinessFixture();
    f.contents.set('/home/agent/agent.status', '');
    const sleep = f.dependencies.sleep;
    f.dependencies.sleep = async ms => { await sleep(ms); change(f); };
    await assert.rejects(async () => run(f.dependencies), expected);
    assert.equal(f.elapsed(), 250);
    assertNoDNSWrites(f);
  });
}

test('rechecks policy after Initialized before DNS writes', async () => {
  const f = readinessFixture();
  const read = f.dependencies.files.readFileSync;
  f.dependencies.files.readFileSync = file => {
    if (file === '/home/agent/agent.status') {
      f.contents.set('/home/agent/agent.json', '{"egress_policy":"audit"}');
    }
    return read(file);
  };
  await assert.rejects(async () => run(f.dependencies), /block mode/);
  assertNoDNSWrites(f);
});
