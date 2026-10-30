const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');

function resolverConfig(original) {
  const nameservers = [...original.matchAll(/^\s*nameserver\s+(\S+)/gm)].map(match => match[1]);
  if (!['127.0.0.100', '127.0.0.53', '127.0.0.1'].includes(nameservers[0])) {
    throw new Error('Unexpected host DNS configuration; refusing automatic repair');
  }
  return original.split(/\r?\n/).filter(line => !/^\s*nameserver\s/.test(line)).join('\n').trimEnd()
    + '\nnameserver 127.0.0.1\n';
}

// The service can be active before its DNS proxy binds or its eBPF block policy
// finishes initializing. Upstream API retries can take longer than 15 seconds;
// allow up to 60 seconds while requiring the same readiness evidence.
const STARTUP_ATTEMPTS = 120;
const STARTUP_INTERVAL_MS = 500;
const sleepSync = ms => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

function ebpfBlockReady(journal) {
  const lines = journal.split('\n');
  const attached = direction => lines.some(line => new RegExp(`\\bINFO Attached CGROUP_${direction} probe module=tracer(?:\\s|$)`).test(line));
  const mode = lines.findLastIndex(line => /\bINFO egress mode info module=tracer\b/.test(line));
  const applied = lines.findLastIndex(line => /\bINFO SetBlockMode module=tracer\b/.test(line));
  return attached('EGRESS') && attached('INGRESS') && mode >= 0
    && /\bpolicyMgr\.mode=BLOCK(?:\s|$)/.test(lines[mode]) && applied > mode;
}

function initializedEbpfAgent(files, config) {
  if (typeof config.correlation_id !== 'string' || !config.correlation_id) return false;
  let status, records;
  try {
    status = files.readFileSync('/home/agent/agent.status', 'utf8').trim();
    records = files.readFileSync('/home/agent/agent.log', 'utf8').split(/\r?\n/)
      .map(line => line.includes('GMT:') ? line.slice(line.indexOf('GMT:') + 4) : '');
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
  // The pinned action can select AgentAlpha when its tier lookup times out.
  // Its eBPF block mode keeps host DNS; it does not run the community DNS proxy.
  // Require initialization and policy evidence for this job, not an audit line
  // containing a command or stale evidence from a different invocation.
  return status === 'Initialized'
    && records.some(record => record.startsWith('[buildInfo] name=AgentAlpha '))
    && records.includes(`Step Security Job Correlation ID: ${config.correlation_id}`)
    && records.some(record => /^Initial Feature flags: \{[^}]*\bNetMode:ebpf(?:\s|\})/.test(record))
    && records.includes('[tracerManager] block-policy applied');
}

function run({ files = fs, exec = execFileSync, env = process.env, platform = process.platform, log = console.log, cleanup = false, sleep = sleepSync } = {}) {
  const sudo = (...args) => exec('sudo', ['-n', ...args], { encoding: 'utf8' });
  const active = () => {
    // Do not confuse a failed sudo invocation with a stopped agent.
    const state = exec('systemctl', ['show', 'agent', '--property=ActiveState', '--value'], { encoding: 'utf8' }).trim();
    if (!['active', 'inactive', 'failed'].includes(state)) throw new Error(`Unexpected agent state: ${state}`);
    return state === 'active';
  };
  if (cleanup) {
    if (!env.STATE_dnsBackup) return;
    if (active()) {
      log('StepSecurity remains active; retain its resolver for cleanup and job reporting.');
      return;
    }
    sudo('cp', '-a', '--remove-destination', path.join(env.STATE_dnsBackup, 'original'), '/etc/resolv.conf');
    log('Agent stopped; restored original host DNS.');
    return;
  }
  if (env['INPUT_DNS-REPAIR'] === 'false') {
    log('DNS repair disabled for this invocation.');
    return;
  }
  if (platform !== 'linux') throw new Error('This diagnostic repair supports Linux only');
  if (!active()) throw new Error('StepSecurity agent is not active');
  const config = JSON.parse(files.readFileSync('/home/agent/agent.json', 'utf8'));
  if (config.egress_policy !== 'block') throw new Error('StepSecurity block mode is required');
  const invocation = () => exec('systemctl', ['show', 'agent', '--property=InvocationID', '--value'], { encoding: 'utf8' }).trim();
  let lastReadError;
  const readEvidence = read => {
    try { return read(); } catch (error) {
      // Unavailable evidence is not readiness. Keep polling for either mode,
      // but preserve the read failure for diagnostics if neither becomes ready.
      lastReadError = error;
      return undefined;
    }
  };
  const ebpfBlocking = () => {
    // The eBPF agent enforces policy in the kernel without a DNS proxy. Require
    // completed probe attachment and block-policy application from this service
    // invocation only; logs from an older agent must not authorize this one.
    const id = readEvidence(invocation);
    if (id === undefined) return false;
    if (!/^[a-f0-9]{32}$/.test(id)) throw new Error('Invalid StepSecurity service invocation ID');
    const journal = readEvidence(() => sudo('journalctl', '--unit=agent', '--no-pager', '--output=cat', `_SYSTEMD_INVOCATION_ID=${id}`));
    if (journal === undefined || !ebpfBlockReady(journal)) return false;
    if (!active()) throw new Error('StepSecurity agent changed during eBPF readiness check');
    const currentId = readEvidence(invocation);
    if (currentId === undefined) return false;
    if (currentId !== id) throw new Error('StepSecurity agent changed during eBPF readiness check');
    return true;
  };
  const listening = () => sudo('ss', '-lnup', '( sport = :53 )')
    .split('\n').some(line => /127\.0\.0\.1:53\s/.test(line) && /"agent"/.test(line));
  let attempt = 1;
  while (true) {
    if (initializedEbpfAgent(files, config)) {
      log('StepSecurity eBPF block policy initialized for this job; preserving host DNS.');
      return;
    }
    if (listening()) break;
    if (ebpfBlocking()) {
      log('StepSecurity eBPF block enforcement is active; retaining host DNS (no DNS proxy required).');
      return;
    }
    if (attempt >= STARTUP_ATTEMPTS) {
      if (lastReadError) throw lastReadError;
      throw new Error(`StepSecurity is not ready after ${(STARTUP_ATTEMPTS * STARTUP_INTERVAL_MS) / 1000}s: expected an agent DNS listener on 127.0.0.1:53 or an initialized eBPF block policy for this job`);
    }
    attempt++;
    sleep(STARTUP_INTERVAL_MS);
  }
  if (attempt > 1) log(`StepSecurity DNS listener ready after ${attempt} checks.`);
  const original = files.readFileSync('/etc/resolv.conf', 'utf8');
  const repaired = resolverConfig(original);
  log(`Original nameservers: ${[...original.matchAll(/^\s*nameserver\s+(\S+)/gm)].map(match => match[1]).join(', ')}`);
  const backup = files.mkdtempSync(path.join(env.RUNNER_TEMP || os.tmpdir(), 'atmos-dns-'));
  sudo('cp', '-a', '/etc/resolv.conf', path.join(backup, 'original'));
  files.writeFileSync(path.join(backup, 'repaired'), repaired, { mode: 0o644 });
  // Register recovery before changing anything. The runner passes this state
  // to the post invocation, including when a later step fails.
  files.appendFileSync(env.GITHUB_STATE, `dnsBackup=${backup}\n`);
  sudo('cp', '--remove-destination', path.join(backup, 'repaired'), '/etc/resolv.conf');
  const actual = files.readFileSync('/etc/resolv.conf', 'utf8');
  if (actual !== repaired) throw new Error('Host DNS changed during repair');
  log('Host DNS now uses the active StepSecurity resolver at 127.0.0.1; firewall policy unchanged.');
}

module.exports = { resolverConfig, run };
if (require.main === module) {
  try { run(); } catch (error) {
    console.error(`DNS repair failed: ${error.message}`);
    process.exitCode = 1;
  }
}
