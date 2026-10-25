const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { performance } = require('node:perf_hooks');
const { setTimeout: sleep } = require('node:timers/promises');

const initializationTimeout = 60_000;
const initializationPollInterval = 250;

function agentStartTimestamp(exec) {
  // D-Bus exposes the realtime timestamp as uint64 microseconds, without locale formatting.
  const value = exec('busctl', [
    '--system', '--timeout=5s', 'get-property', 'org.freedesktop.systemd1',
    '/org/freedesktop/systemd1/unit/agent_2eservice',
    'org.freedesktop.systemd1.Service', 'ExecMainStartTimestamp',
  ], { encoding: 'utf8', timeout: 6000 }).trim();
  const match = /^t ([0-9]+)$/.exec(value);
  if (!match || BigInt(match[1]) === 0n) throw new Error('Invalid StepSecurity agent start timestamp');
  return BigInt(match[1]) * 1000n;
}

async function waitForInitialization({ files, checkAgent, generation, now, sleep }) {
  const deadline = now() + initializationTimeout;
  const checkDeadline = () => {
    if (now() >= deadline) {
      throw new Error(`StepSecurity initialization did not complete within ${initializationTimeout}ms`);
    }
  };
  checkAgent();
  const startedAt = generation();
  const checkGeneration = () => {
    checkDeadline();
    checkAgent();
    if (generation() !== startedAt) throw new Error('StepSecurity agent restarted during readiness verification');
    checkDeadline();
  };
  while (true) {
    checkGeneration();
    let status;
    let modifiedAt;
    try {
      status = files.readFileSync('/home/agent/agent.status', 'utf8').trim();
      if (status) modifiedAt = files.statSync('/home/agent/agent.status', { bigint: true }).mtimeNs;
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
      status = undefined;
    }
    if (status && !/^(Initialized)+$/.test(status)) {
      throw new Error(`Unexpected agent initialization status: ${status}`);
    }
    if (status && typeof modifiedAt !== 'bigint') throw new Error('Invalid agent readiness file timestamp');
    // The legacy agent appends one token per generation. An old token is not readiness.
    if (status && modifiedAt >= startedAt) {
      checkGeneration();
      return checkGeneration;
    }
    await sleep(Math.min(initializationPollInterval, Math.max(0, deadline - now())));
  }
}

function resolverConfig(original) {
  const nameservers = [...original.matchAll(/^\s*nameserver\s+(\S+)/gm)].map(match => match[1]);
  if (!['127.0.0.100', '127.0.0.53', '127.0.0.1'].includes(nameservers[0])) {
    throw new Error('Unexpected host DNS configuration; refusing automatic repair');
  }
  return original.split(/\r?\n/).filter(line => !/^\s*nameserver\s/.test(line)).join('\n').trimEnd()
    + '\nnameserver 127.0.0.1\n';
}

async function run({ files = fs, exec = execFileSync, env = process.env, platform = process.platform, log = console.log, cleanup = false, now = () => performance.now(), sleep: pause = sleep } = {}) {
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
  const checkAgent = () => {
    if (!active()) throw new Error('StepSecurity agent is not active');
    const config = JSON.parse(files.readFileSync('/home/agent/agent.json', 'utf8'));
    if (config.egress_policy !== 'block') throw new Error('StepSecurity block mode is required');
  };
  // The upstream pre-hook can time out while the agent is still restarting Docker.
  // Do not let subsequent workflow steps overlap that initialization.
  const checkGeneration = await waitForInitialization({
    files, checkAgent, generation: () => agentStartTimestamp(exec), now, sleep: pause,
  });
  checkGeneration();
  log('StepSecurity initialization completed; proceeding with DNS verification.');
  const listeners = sudo('ss', '-lnup', '( sport = :53 )');
  if (!listeners.split('\n').some(line => /127\.0\.0\.1:53\s/.test(line) && /"agent"/.test(line))) {
    throw new Error('StepSecurity is not listening on 127.0.0.1:53');
  }
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
  run().catch(error => {
    console.error(`DNS repair failed: ${error.message}`);
    process.exitCode = 1;
  });
}
