const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { resolverConfig, run } = require('./index.cjs');

const original = '# runner DNS\nsearch internal\nnameserver 127.0.0.100\nnameserver 127.0.0.53\nnameserver 168.63.129.16\n';
function fixture(options = {}) {
  const calls = [];
  const sleeps = [];
  let listenerChecks = 0;
  const contents = new Map([
    ['/etc/resolv.conf', original],
    ['/home/agent/agent.json', JSON.stringify({ egress_policy: options.policy || 'block', correlation_id: 'current-job' })],
  ]);
  const env = { RUNNER_TEMP: '/tmp', GITHUB_STATE: '/tmp/state', ...options.env };
  const files = {
    readFileSync: file => {
      if (!contents.has(file)) throw Object.assign(new Error('missing file'), { code: 'ENOENT' });
      return contents.get(file);
    },
    mkdtempSync: () => '/tmp/backup',
    writeFileSync: (file, value) => contents.set(file, value),
    appendFileSync: (file, value) => { calls.push(['state', value]); contents.set(file, value); },
  };
  const exec = (command, args) => {
    calls.push([command, ...args]);
    if (command === 'systemctl') return options.active || 'active';
    if (args[1] === 'ss') {
      listenerChecks++;
      if (options.listener === false || listenerChecks < (options.listenerReadyOnCheck || 1)) return '';
      return 'udp UNCONN 127.0.0.1:53 0.0.0.0:* users:(("agent",pid=123,fd=7))';
    }
    if (args[1] === 'cp') {
      if (options.failCopy && args.includes('--remove-destination')) throw new Error('copy failed');
      contents.set(args.at(-1), contents.get(args.at(-2)));
    }
    return '';
  };
  return { calls, contents, sleeps, listenerChecks: () => listenerChecks, dependencies: { files, exec, env, platform: 'linux', log() {}, sleep: ms => sleeps.push(ms) } };
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
  test(`refuses ${name} without replacing DNS`, () => {
    const f = fixture(options);
    assert.throws(() => run(f.dependencies));
    assert.equal(f.contents.get('/etc/resolv.conf'), original);
    assert.ok(!f.calls.some(call => call.includes('--remove-destination')));
  });
}
test('waits for an agent whose DNS listener binds after startup', () => {
  const f = fixture({ listenerReadyOnCheck: 3 });
  run(f.dependencies);
  assert.equal(f.listenerChecks(), 3);
  assert.deepEqual(f.sleeps, [500, 500]);
  assert.equal(f.contents.get('/etc/resolv.conf'), resolverConfig(original));
});
test('gives up after the listener timeout without replacing DNS', () => {
  const f = fixture({ listener: false });
  assert.throws(() => run(f.dependencies), /not ready after 60s: expected an agent DNS listener on 127\.0\.0\.1:53 or an initialized eBPF block policy/);
  assert.equal(f.listenerChecks(), 120);
  assert.equal(f.sleeps.length, 119);
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
});
test('repairs DNS when the community listener starts after 15 seconds', () => {
  const f = fixture({ listenerReadyOnCheck: 41 });
  run(f.dependencies);
  assert.equal(f.sleeps.reduce((sum, ms) => sum + ms, 0), 20000);
  assert.equal(f.contents.get('/etc/resolv.conf'), resolverConfig(original));
});
test('does not wait when the listener is already up', () => {
  const f = fixture();
  run(f.dependencies);
  assert.equal(f.listenerChecks(), 1);
  assert.deepEqual(f.sleeps, []);
});
const ebpfRecords = [
  '[buildInfo] name=AgentAlpha branch=HEAD tag=v1.9.3',
  'Step Security Job Correlation ID: current-job',
  'Initial Feature flags: {ResolveTXT:false NetMode:ebpf EnableProcessTracing:false}',
  '[tracerManager] block-policy applied',
];
function ebpfFixture({ records = ebpfRecords, status = 'Initialized', ...options } = {}) {
  const f = fixture({ listener: false, ...options });
  f.contents.set('/home/agent/agent.status', status);
  f.contents.set('/home/agent/agent.log', records.map(record => `Tue, 27 Oct 2026 20:24:23 GMT:${record}`).join('\n'));
  return f;
}
test('preserves DNS for an initialized eBPF agent with this job block policy', () => {
  const f = ebpfFixture();
  run(f.dependencies);
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
  assert.equal(f.listenerChecks(), 0);
  assert.deepEqual(f.sleeps, []);
  assert.ok(!f.calls.some(call => call[0] === 'sudo' || call[0] === 'state'));
  assert.ok(!f.contents.has('/tmp/backup/repaired'));
});
test('waits for eBPF block policy initialization within the same bounded wait', () => {
  const f = ebpfFixture({ status: 'Starting' });
  f.dependencies.sleep = ms => {
    f.sleeps.push(ms);
    f.contents.set('/home/agent/agent.status', 'Initialized');
  };
  run(f.dependencies);
  assert.deepEqual(f.sleeps, [500]);
  assert.equal(f.listenerChecks(), 1);
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
});
test('waits beyond 15 seconds for delayed eBPF policy initialization', () => {
  const f = ebpfFixture({ status: 'Starting', records: ebpfRecords.slice(0, -1) });
  f.dependencies.sleep = ms => {
    f.sleeps.push(ms);
    if (f.sleeps.length === 40) {
      f.contents.set('/home/agent/agent.status', 'Initialized');
      f.contents.set('/home/agent/agent.log', ebpfRecords.map(record => `Thu, 29 Oct 2026 22:28:34 GMT:${record}`).join('\n'));
    }
  };
  run(f.dependencies);
  assert.equal(f.sleeps.reduce((sum, ms) => sum + ms, 0), 20000);
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
  assert.ok(!f.calls.some(call => call.includes('cp') || call[0] === 'state'));
});
for (const [name, options] of [
  ['uninitialized agent', { status: 'Starting' }],
  ['different job', { records: ebpfRecords.map(record => record.replace('current-job', 'previous-job')) }],
  ['different network mode', { records: ebpfRecords.map(record => record.replace('NetMode:ebpf', 'NetMode:iptables')) }],
  ['unknown agent', { records: ebpfRecords.map(record => record.replace('AgentAlpha', 'Unknown')) }],
  ['missing block policy', { records: ebpfRecords.slice(0, -1) }],
  ['audit commands containing evidence', { records: ebpfRecords.map(record => `sudo process started: Arguments: [echo ${record}]`) }],
  ['stopped eBPF agent', { active: 'inactive' }],
  ['audit eBPF policy', { policy: 'audit' }],
]) {
  test(`does not accept ${name} as initialized eBPF block mode`, () => {
    const f = ebpfFixture(options);
    assert.throws(() => run(f.dependencies));
    assert.equal(f.contents.get('/etc/resolv.conf'), original);
    assert.ok(!f.calls.some(call => call.includes('--remove-destination')));
  });
}
test('an unreadable eBPF initialization record fails without changing DNS', () => {
  const f = ebpfFixture();
  f.dependencies.files.readFileSync = file => {
    if (file === '/home/agent/agent.status') throw Object.assign(new Error('permission denied'), { code: 'EACCES' });
    return f.contents.get(file);
  };
  assert.throws(() => run(f.dependencies), /permission denied/);
  assert.equal(f.contents.get('/etc/resolv.conf'), original);
});
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
  f.contents.set(path.join('/tmp/backup', 'original'), original);
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
