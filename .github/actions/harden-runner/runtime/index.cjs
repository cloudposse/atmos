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

// The agent reports active before its DNS proxy binds 127.0.0.1:53, so a single
// listener check right after startup races it. Poll for up to 15 seconds.
const LISTENER_ATTEMPTS = 30;
const LISTENER_INTERVAL_MS = 500;
const sleepSync = ms => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

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
  const listening = () => sudo('ss', '-lnup', '( sport = :53 )')
    .split('\n').some(line => /127\.0\.0\.1:53\s/.test(line) && /"agent"/.test(line));
  let attempt = 1;
  while (!listening()) {
    if (attempt >= LISTENER_ATTEMPTS) {
      throw new Error(`StepSecurity is not listening on 127.0.0.1:53 after ${(LISTENER_ATTEMPTS * LISTENER_INTERVAL_MS) / 1000}s`);
    }
    attempt++;
    sleep(LISTENER_INTERVAL_MS);
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
