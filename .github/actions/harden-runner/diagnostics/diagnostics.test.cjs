const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createDiagnostics, evaluate, hasRejection, IMAGE, URLS } = require('./diagnostics.cjs');
const { main } = require('./index.cjs');
const { execute } = require('./command.cjs');

function fixture(t, overrides = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'atmos-network-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const env = { RUNNER_TEMP: root, RUNNER_OS: 'Linux', GITHUB_RUN_ID: '42', GITHUB_RUN_ATTEMPT: '1',
    GITHUB_STEP_SUMMARY: path.join(root, 'summary'), ...overrides.env };
  const calls = [], waits = [], output = [];
  const state = { canary: '', reads: 0 };
  function run(command, args, timeout) {
    calls.push({ command, args, timeout });
    const custom = overrides.run?.(command, args, state);
    if (custom) return custom;
    if (command === 'sudo' && args[1] === 'wc') return { code: 0, stdout: '10 /home/agent/agent.log\n', stderr: '' };
    if (command === 'sudo' && args[1] === 'tail') {
      assert.deepEqual(args, ['-n', 'tail', '-n', '+11', '/home/agent/agent.log']);
      return { code: 0, stdout: `Thu, 22 Oct 2026 22:37:57 GMT:domain not allowed: ${state.canary}.\n`, stderr: '' };
    }
    const url = args.at(-1);
    if (typeof url === 'string' && url.startsWith('https://')) {
      const host = new URL(url).hostname;
      if (host.endsWith('.example.com')) { state.canary = host; return { code: 7, stdout: '', stderr: 'refused' }; }
      return { code: 0, stdout: 'ip=192.0.2.1 http=404', stderr: '' };
    }
    return { code: 0, stdout: command === 'systemctl' ? 'active\n' : '', stderr: '' };
  }
  const files = {
    ...fs,
    readFileSync(file, encoding) {
      if (file === '/etc/resolv.conf') return 'nameserver 127.0.0.1\n';
      if (file === '/etc/systemd/resolved.conf') return 'DNS=127.0.0.1\n';
      if (file === '/etc/docker/daemon.json') return '{"dns":["172.17.0.1"],"secret":"must-not-leak"}';
      return fs.readFileSync(file, encoding);
    },
    ...overrides.files,
  };
  const api = createDiagnostics({ files, run, env, wait: async ms => { waits.push(ms); }, log: value => output.push(value) });
  const directory = path.join(root, 'network-diagnostics');
  return { ...api, root, directory, calls, waits, output, env };
}

const record = (phase, findings = [], complete = true) => ({ phase, findings, complete });
const ebpfDrop = 'Thu, 29 Oct 2026 22:50:10 GMT:[dns-request-dropped] time=2026-10-29 22:50:10.740606539 +0000 UTC m=+20.870861424 dnsID=38544 questionType=A pid=3350 exe=/usr/bin/curl domain=probe.example.com. matchedPolicy=NONE';
for (const [name, line, expected] of [
  ['exact', 'Thu, 22 Oct 2026 22:37:57 GMT:domain not allowed: probe.example.com.', true],
  ['command audit', 'Thu, 22 Oct 2026 22:37:57 GMT:sudo [grep domain not allowed: probe.example.com.]', false],
  ['other domain', 'Thu, 22 Oct 2026 22:37:57 GMT:domain not allowed: old.example.com.', false],
  ['suffix', 'Thu, 22 Oct 2026 22:37:57 GMT:domain not allowed: probe.example.com.evil.', false],
  ['no record prefix', 'domain not allowed: probe.example.com.', false],
  ['eBPF dropped query', ebpfDrop, true],
  ['eBPF container AAAA query', ebpfDrop.replace('questionType=A', 'questionType=AAAA').replace('/usr/bin/curl', '/bin/busybox'), true],
  ['eBPF allowed query', ebpfDrop.replace('[dns-request-dropped]', '[dns-request]'), false],
  ['eBPF other domain', ebpfDrop.replace('domain=probe.example.com.', 'domain=old.example.com.'), false],
  ['eBPF domain suffix', ebpfDrop.replace('domain=probe.example.com.', 'domain=probe.example.com.evil.'), false],
  ['eBPF another policy', ebpfDrop.replace('matchedPolicy=NONE', 'matchedPolicy=WILDCARD_DOMAIN'), false],
  ['eBPF command audit', ebpfDrop.replace('GMT:', 'GMT:sudo process started: Arguments: [echo ') + ']', false],
  ['eBPF trailing audit text', ebpfDrop + ' echoed by another process', false],
]) test(`rejection record: ${name}`, () => assert.equal(hasRejection(line, 'probe.example.com'), expected));

test('fresh eBPF rejection records confirm both host and container enforcement', async t => {
  const f = fixture(t, { run(command, args, state) {
    if (command === 'sudo' && args[1] === 'tail') {
      assert.deepEqual(args, ['-n', 'tail', '-n', '+11', '/home/agent/agent.log']);
      return { code: 0, stdout: ebpfDrop.replace('probe.example.com', state.canary) + '\n', stderr: '' };
    }
  } });
  const result = await f.perform('probe', { phase: 'repaired' });
  assert.deepEqual(result.findings, []);
  await f.perform('capture');
  await f.perform('report', { expectRepair: true });
  const evidence = fs.readFileSync(path.join(f.directory, 'enforcement.log'), 'utf8').trim().split('\n');
  assert.equal(evidence.length, 2);
  assert.ok(evidence.every(line => line.endsWith('rejection_confirmed=true')));
});

test('before/after recovery passes while preserving failed original probes', () => {
  assert.deepEqual(evaluate([record('hardened', ['incomplete', 'connectivity-failed']), record('repaired')], true, false), ['repaired']);
});
for (const [name, results, repair, first] of [
  ['denied destination reached before repair', [record('hardened', ['policy-failed']), record('repaired')], true, false],
  ['final connection failure', [record('repaired', ['connectivity-failed'])], true, false],
  ['incomplete final evidence', [record('repaired', ['incomplete'])], true, false],
  ['interrupted final probes', [record('repaired', [], false)], true, false],
  ['missing final probes', [], true, false],
  ['repair-first initial failure', [record('hardened', ['connectivity-failed']), record('repaired')], true, true],
  ['observation failure', [record('hardened', ['connectivity-failed'])], false, false],
  ['invalid result phase', [record('unknown')], false, false],
  ['invalid result findings', [record('hardened', ['unknown'])], false, false],
  ['invalid completion value', [record('hardened', [], 'true')], false, false],
]) test(`report blocks ${name}`, () => assert.throws(() => evaluate(results, repair, first)));

test('observation and repair-first modes require their respective phases', () => {
  assert.deepEqual(evaluate([record('hardened')], false, false), ['hardened']);
  assert.deepEqual(evaluate([record('hardened'), record('repaired')], true, true), ['hardened', 'repaired']);
});

test('full lifecycle probes host/container, captures safe evidence, and reports success', async t => {
  const f = fixture(t);
  await f.perform('prepare');
  const original = await f.perform('probe', { phase: 'hardened' });
  const repaired = await f.perform('probe', { phase: 'repaired' });
  await f.perform('capture');
  await f.perform('report', { expectRepair: true, repairFirst: true });
  assert.equal(original.probes.length, 12);
  assert.equal(repaired.probes.length, 12);
  assert.deepEqual(repaired.probes.slice(0, 5).map(p => p.url), URLS);
  assert.equal(repaired.probes[11].url, 'https://repaired-container-42-1.example.com/');
  assert.deepEqual(repaired.findings, []);
  // Only the repaired phase waits for Docker's resolver cache when the image is already present.
  assert.equal(f.waits.filter(ms => ms === 6000).length, 1);
  const evidence = fs.readFileSync(path.join(f.directory, 'enforcement.log'), 'utf8').trim().split('\n');
  assert.equal(evidence.length, 4);
  assert.equal(new Set(evidence.map(line => line.split('domain=')[1].split(' ')[0])).size, 4);
  assert.ok(evidence.every(line => line.endsWith('rejection_confirmed=true')));
  const files = fs.readdirSync(f.directory);
  assert.ok(!files.some(name => ['agent.log', 'agent.status', 'agent.json'].includes(name)));
  for (const name of files) assert.ok(!fs.readFileSync(path.join(f.directory, name), 'utf8').includes('must-not-leak'));
  const containers = f.calls.filter(call => call.command === 'docker' && call.args[0] === 'run');
  assert.ok(containers.length > 0);
  for (const call of containers) {
    assert.ok(call.args.includes('--read-only'));
    assert.ok(call.args.includes('no-new-privileges'));
    assert.ok(call.args.includes(IMAGE));
  }
  assert.ok(f.calls.every(call => call.timeout > 0 && call.timeout <= 90000));
});

test('hardened probes retry a failed image download after the resolver cache window', async t => {
  // A pull that ran right after a DNS repair can fail; the hardened phase must not report its
  // container probes incomplete when a retry after the cache window succeeds.
  const f = fixture(t, { run(command, args, state) {
    if (command === 'docker' && args[0] === 'pull') {
      state.pulls = (state.pulls || 0) + 1;
      return { code: state.pulls === 1 ? 1 : 0, stdout: '', stderr: state.pulls === 1 ? 'lookup failed' : '' };
    }
    if (command === 'docker' && args[0] === 'image') return { code: state.pulls > 1 ? 0 : 1, stdout: '', stderr: '' };
  } });
  await f.perform('prepare');
  assert.ok(f.output.some(line => line.includes('download failed')));
  const original = await f.perform('probe', { phase: 'hardened' });
  assert.deepEqual(f.waits.filter(ms => ms === 6000), [6000]);
  assert.equal(f.calls.filter(call => call.command === 'docker' && call.args[0] === 'pull').length, 2);
  assert.equal(original.probes.length, 12);
  assert.deepEqual(original.findings, []);
  assert.ok(!f.output.some(line => line.includes('Container probes unavailable')));
});

test('hardened probes stay incomplete when the image download keeps failing', async t => {
  const f = fixture(t, { run(command, args) {
    if (command === 'docker' && args[0] === 'pull') return { code: 1, stdout: '', stderr: 'lookup failed' };
    if (command === 'docker' && args[0] === 'image') return { code: 1, stdout: '', stderr: '' };
  } });
  await f.perform('prepare');
  const original = await f.perform('probe', { phase: 'hardened' });
  assert.equal(original.probes.length, 6);
  assert.deepEqual(original.findings, ['incomplete']);
  await assert.rejects(f.perform('report', { expectRepair: true, repairFirst: true }), /hardened: incomplete/);
});

for (const mode of ['stale', 'missing-log', 'tail-failed', 'malformed-count', 'late']) {
  test(`canary evidence handles ${mode}`, async t => {
    const f = fixture(t, { run(command, args, state) {
      if (command === 'sudo' && args[1] === 'wc') {
        if (mode === 'missing-log') return { code: 1, stdout: '', stderr: 'denied' };
        if (mode === 'malformed-count') return { code: 0, stdout: 'invalid', stderr: '' };
      }
      if (command === 'sudo' && args[1] === 'tail') {
        state.reads++;
        if (mode === 'late' && state.reads % 2 === 0) return;
        return { code: mode === 'tail-failed' ? 1 : 0, stdout: '', stderr: '' };
      }
    } });
    const result = await f.perform('probe', { phase: 'repaired' });
    assert.equal(result.findings.includes('incomplete'), mode !== 'late');
    if (mode === 'late') assert.ok(f.waits.includes(500));
  });
}

test('reachable canary fails the diagnostic report', async t => {
  const reachable = new Set([
    'https://hardened-host-42-1.example.com/',
    'https://hardened-container-42-1.example.com/',
  ]);
  const f = fixture(t, { run(command, args) {
    if (['curl', 'docker'].includes(command) && reachable.has(args.at(-1))) {
      return { code: 0, stdout: 'ip=192.0.2.2 http=200', stderr: '' };
    }
  } });
  const result = await f.perform('probe', { phase: 'hardened' });
  assert.deepEqual(result.findings, ['policy-failed']);
  await assert.rejects(f.perform('report'), /Unlisted destination/);
});

test('missing image retains host results but makes final evidence incomplete', async t => {
  const f = fixture(t, { run(command, args) {
    if (command === 'docker' && ['pull', 'image'].includes(args[0])) return { code: 1, stdout: '', stderr: 'refused' };
  } });
  await f.perform('prepare');
  const result = await f.perform('probe', { phase: 'repaired' });
  assert.equal(result.probes.length, 6);
  assert.deepEqual(result.findings, ['incomplete']);
  await assert.rejects(f.perform('report', { expectRepair: true }), /incomplete/);
});

test('allowed destination failure is retained and reported', async t => {
  const f = fixture(t, { run(command, args) {
    if (command === 'curl' && args.at(-1) === URLS[0]) return { code: 7, stdout: '', stderr: 'refused' };
  } });
  const result = await f.perform('probe', { phase: 'hardened' });
  assert.deepEqual(result.findings, ['connectivity-failed']);
  await assert.rejects(f.perform('report'), /connectivity-failed/);
});

test('unexpected command exception leaves an incomplete record', async t => {
  const f = fixture(t, { run(command) { if (command === 'curl') throw new Error('interrupted'); } });
  await assert.rejects(f.perform('probe', { phase: 'repaired' }), /interrupted/);
  const result = JSON.parse(fs.readFileSync(path.join(f.directory, 'repaired.json'), 'utf8'));
  assert.equal(result.complete, false);
  await assert.rejects(f.perform('report', { expectRepair: true }), /interrupted/);
});

test('capture runs even when preparation never completed; inactive agent fails reporting', async t => {
  const f = fixture(t, { run(command) {
    if (command === 'systemctl') return { code: 3, stdout: 'inactive\n', stderr: '' };
  } });
  await f.perform('capture');
  assert.equal(fs.readFileSync(path.join(f.directory, 'agent-state.txt'), 'utf8'), 'inactive\n');
  await assert.rejects(f.perform('report'), /agent is not active/);
});

test('rejects invalid operations, phases, run IDs, and missing temporary directory', async t => {
  const f = fixture(t, { env: { GITHUB_RUN_ID: 'bad id' } });
  await assert.rejects(f.perform('invalid'), /Invalid diagnostic operation/);
  await assert.rejects(f.perform('probe', { phase: '../escape' }), /Invalid probe phase/);
  await assert.rejects(f.perform('probe', { phase: 'hardened' }), /Numeric GitHub/);
  assert.throws(() => createDiagnostics({ env: {} }), /RUNNER_TEMP/);
});

test('entrypoint parses the real hyphenated action input environment variables', async () => {
  let received;
  await main({ RUNNER_OS: 'Linux', INPUT_OPERATION: 'report', 'INPUT_EXPECT-REPAIR': 'true', 'INPUT_REPAIR-FIRST': 'true' },
    () => ({ perform: async (...args) => { received = args; } }));
  assert.deepEqual(received, ['report', { phase: undefined, expectRepair: true, repairFirst: true }]);
  await assert.rejects(main({ RUNNER_OS: 'Windows' }), /Linux/);
  await assert.rejects(main({ RUNNER_OS: 'Linux', RUNNER_TEMP: '/tmp', 'INPUT_EXPECT-REPAIR': 'yes' }), /Invalid boolean/);
});

test('command adapter preserves stdout, nonzero exits, launch errors, and timeouts', () => {
  assert.deepEqual(execute(process.execPath, ['-e', 'process.stdout.write("ok")']), { code: 0, stdout: 'ok', stderr: '' });
  assert.equal(execute(process.execPath, ['-e', 'process.exit(7)']).code, 7);
  assert.notEqual(execute(path.join(os.tmpdir(), 'atmos-nonexistent-command'), []).code, 0);
  assert.notEqual(execute(process.execPath, ['-e', 'setTimeout(() => {}, 10000)'], 50).code, 0);
});

test('step summary explains recovered initial failures and confirms enforcement', async t => {
  let fail = true;
  const f = fixture(t, { run(command, args) {
    if (fail && command === 'curl' && args.at(-1) === URLS[0]) return { code: 7, stdout: '', stderr: 'refused' };
  } });
  await f.perform('probe', { phase: 'hardened' });
  fail = false;
  await f.perform('probe', { phase: 'repaired' });
  await f.perform('report', { expectRepair: true });
  const summary = fs.readFileSync(f.env.GITHUB_STEP_SUMMARY, 'utf8');
  assert.match(summary, /## Initial connectivity/);
  assert.match(summary, /## After DNS repair/);
  assert.match(summary, /blocking confirmed/);
  assert.match(summary, /\*\*Passed\.\*\*/);
  assert.match(summary, /Original DNS failed; recovery verified/);
});

test('step summary explains failed final evidence', async t => {
  const f = fixture(t);
  await assert.rejects(f.perform('report', { expectRepair: true }), /Missing or interrupted repaired/);
  const summary = fs.readFileSync(f.env.GITHUB_STEP_SUMMARY, 'utf8');
  assert.match(summary, /\*\*Failed:\*\* Missing or interrupted repaired probes/);
});

test('unreadable snapshot files do not abort transport probes', async t => {
  const f = fixture(t, { files: { readFileSync() { throw new Error('unavailable'); } } });
  const result = await f.perform('probe', { phase: 'hardened' });
  assert.equal(result.complete, true);
  assert.deepEqual(result.findings, []);
  assert.ok(f.output.some(line => line.includes('(unavailable)')));
});
