const fs = require('node:fs');
const path = require('node:path');
const { setTimeout: sleep } = require('node:timers/promises');
const { execute } = require('./command.cjs');

const IMAGE = 'curlimages/curl:8.12.1@sha256:94e9e444bcba979c2ea12e27ae39bee4cd10bc7041a472c4727a558e213744e6';
const URLS = [
  'https://github.com/',
  'https://api.github.com/meta',
  'https://codeload.github.com/actions/download-artifact/tar.gz/d3f86a106a0bac45b974a628896c90dbdf5c8093',
  'https://proxy.golang.org/golang.org/x/text/@v/list',
  'https://release-assets.githubusercontent.com/',
];
const PHASES = ['hardened', 'repaired'];
const FINDINGS = ['policy-failed', 'incomplete', 'connectivity-failed'];
const AGENT_LOG = '/home/agent/agent.log';

// Match the actual record, never a command audit line containing the search text.
function hasRejection(text, domain) {
  return text.split(/\r?\n/).some(line => {
    const boundary = line.indexOf('GMT:');
    if (boundary < 0) return false;
    const record = line.slice(boundary + 4);
    if (record === `domain not allowed: ${domain}.`) return true;
    // AgentAlpha records rejected DNS queries as structured eBPF events rather
    // than the community proxy's message. Require a dropped query with the exact
    // canary domain and no matching allow policy; ordinary DNS requests are not
    // evidence of enforcement.
    const dropped = record.match(/^\[dns-request-dropped\] .+ domain=(\S+) matchedPolicy=NONE$/);
    return dropped?.[1] === `${domain}.`;
  });
}

function evaluate(results, expectRepair, repairFirst) {
  for (const result of results) {
    if (!PHASES.includes(result.phase) || typeof result.complete !== 'boolean'
      || !Array.isArray(result.findings) || result.findings.some(item => !FINDINGS.includes(item))) {
      throw new Error('Invalid diagnostic result');
    }
    if (result.findings.includes('policy-failed')) throw new Error('Unlisted destination was reachable');
  }
  const required = expectRepair ? (repairFirst ? PHASES : ['repaired']) : ['hardened'];
  for (const phase of required) {
    const result = results.find(item => item.phase === phase);
    if (!result?.complete) throw new Error(`Missing or interrupted ${phase} probes`);
    if (result.findings.length) throw new Error(`${phase}: ${result.findings.join(', ')}`);
  }
  return required;
}

function createDiagnostics({ files = fs, run = execute, wait = sleep, env = process.env, log = console.log } = {}) {
  if (!env.RUNNER_TEMP) throw new Error('RUNNER_TEMP is required');
  const directory = path.join(env.RUNNER_TEMP, 'network-diagnostics');
  const write = (name, value) => files.writeFileSync(path.join(directory, name), value);
  const append = (name, value) => files.appendFileSync(path.join(directory, name), value);
  const summary = value => { if (env.GITHUB_STEP_SUMMARY) files.appendFileSync(env.GITHUB_STEP_SUMMARY, value); };
  const record = (name, value) => { append(name, `${value}\n`); log(value); };
  const sudo = (args, timeout) => run('sudo', ['-n', ...args], timeout);
  const container = args => ['run', '--rm', '--pull=never', '--network', 'bridge', '--cap-drop', 'ALL',
    '--security-opt', 'no-new-privileges', '--read-only', ...args];
  const read = file => { try { return files.readFileSync(file, 'utf8'); } catch { return '(unavailable)'; } };

  function pull() {
    const result = run('docker', ['pull', IMAGE], 90000);
    log(result.code === 0 ? 'Pinned probe image downloaded.' : '::warning::Probe image download failed; host probes will still run.');
  }
  function snapshot(name) {
    for (const file of ['/etc/resolv.conf', '/etc/systemd/resolved.conf']) record(name, `--- ${file}\n${read(file)}`);
    // Docker resolver state is useful; arbitrary daemon configuration is not.
    try {
      const config = JSON.parse(files.readFileSync('/etc/docker/daemon.json', 'utf8'));
      record(name, `Docker DNS: ${JSON.stringify(config.dns || [])}`);
    } catch { record(name, 'Docker DNS configuration unavailable.'); }
    for (const [command, args] of [
      ['resolvectl', ['status']],
      ['sudo', ['-n', 'ss', '-lntup', '( sport = :53 )']],
      ['systemctl', ['is-active', 'agent']],
      ['sudo', ['-n', 'iptables-save']],
      ['sudo', ['-n', 'ip6tables-save']],
    ]) {
      const result = run(command, args, 5000);
      record(name, `--- ${command} ${args.join(' ')} (exit ${result.code})\n${result.stdout}${result.stderr}`);
    }
  }
  function logStart() {
    const result = sudo(['wc', '-l', AGENT_LOG], 5000);
    const match = result.stdout.trim().match(/^(\d+)(?:\s|$)/);
    if (result.code !== 0 || !match) return null;
    const count = Number(match[1]);
    return Number.isSafeInteger(count + 1) ? count + 1 : null;
  }
  async function rejection(domain, start) {
    if (start === null) return false;
    for (let attempt = 0; attempt < 4; attempt++) {
      const result = sudo(['tail', '-n', `+${start}`, AGENT_LOG], 5000);
      if (result.code === 0 && hasRejection(result.stdout, domain)) return true;
      if (attempt < 3) await wait(500);
    }
    return false;
  }
  async function probe(phase) {
    if (!PHASES.includes(phase)) throw new Error(`Invalid probe phase: ${phase}`);
    if (!/^\d+$/.test(env.GITHUB_RUN_ID || '') || !/^\d+$/.test(env.GITHUB_RUN_ATTEMPT || '')) {
      throw new Error('Numeric GitHub run ID and attempt are required');
    }
    const result = { phase, complete: false, findings: [], probes: [] };
    const finding = name => { if (!result.findings.includes(name)) result.findings.push(name); };
    const filename = `${phase}.log`;
    // Clear the completion record before work, including interrupted retries.
    write(`${phase}.json`, JSON.stringify(result));
    write(filename, `Phase: ${phase}\nImage: ${env.ImageOS || 'unknown'} ${env.ImageVersion || 'unknown'}\n`);
    try {
      const imageAvailable = () => run('docker', ['image', 'inspect', IMAGE], 5000).code === 0;
      // Docker's Go resolver can cache resolv.conf for five seconds, so a download that ran
      // right after a DNS repair can fail before the daemon sees the repaired resolver. Repaired
      // probes always wait and download again; any phase retries when the image is still missing,
      // so a repair-first run does not report its initial probes incomplete over that race.
      if (phase === 'repaired' || !imageAvailable()) {
        await wait(6000);
        pull();
      }
      snapshot(filename);
      const imageReady = imageAvailable();
      if (imageReady) {
        const dns = run('docker', container(['--entrypoint', 'cat', IMAGE, '/etc/resolv.conf']), 10000);
        record(filename, `Container DNS (exit ${dns.code}):\n${dns.stdout}${dns.stderr}`);
      }
      const enforcementNotes = [];
      summary(`\n## ${phase === 'repaired' ? 'After DNS repair' : 'Initial connectivity'}\n\n| Phase | Client | URL | Exit | Connection |\n|---|---|---|---|---|\n`);
      for (const client of ['host', 'container']) {
        if (client === 'container' && !imageReady) {
          finding('incomplete');
          record(filename, 'Container probes unavailable: pinned image was not downloaded.');
          continue;
        }
        const canary = `${phase}-${client}-${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}.example.com`;
        const deniedURL = `https://${canary}/`;
        for (const url of [...URLS, deniedURL]) {
          const denied = url === deniedURL;
          const start = denied ? logStart() : null;
          const args = ['--noproxy', '*', '--silent', '--show-error', '--head', '--connect-timeout', '3',
            '--max-time', '8', '--output', '/dev/null', '--write-out', 'ip=%{remote_ip} http=%{http_code}', url];
          const response = client === 'host' ? run('curl', args, 10000)
            : run('docker', container([IMAGE, ...args]), 15000);
          const row = { client, url, exit: response.code, connection: response.stdout.trim() };
          if (response.stderr) record(filename, response.stderr.trim());
          result.probes.push(row);
          record(filename, `${client} ${url} exit=${row.exit} ${row.connection}`);
          summary(`| ${phase} | ${client} | ${url} | ${row.exit} | ${row.connection} |\n`);
          if (denied) {
            const confirmed = response.code !== 0 && await rejection(canary, start);
            if (response.code === 0) finding('policy-failed');
            else if (!confirmed) finding('incomplete');
            append('enforcement.log', `${phase} ${client} domain=${canary} curl_exit=${response.code} rejection_confirmed=${confirmed}\n`);
            enforcementNotes.push(`- ${client} canary: **${confirmed ? 'blocking confirmed' : 'blocking NOT confirmed'}** (${canary}).`);
          } else if (response.code !== 0) finding('connectivity-failed');
        }
      }
      summary(`\n${enforcementNotes.join('\n')}\n`);
      // Query resolvers after transport probes so these lookups cannot prime them.
      for (const url of URLS) {
        const domain = new URL(url).hostname;
        record(filename, `--- DNS: ${domain}\n${run('getent', ['ahostsv4', domain], 5000).stdout}`);
        for (const resolver of ['127.0.0.100', '127.0.0.53', '127.0.0.1']) {
          const answer = run('dig', ['+time=1', '+tries=1', '+noall', '+answer', `@${resolver}`, domain, 'A'], 2000);
          record(filename, `${resolver}: ${answer.stdout}`);
        }
      }
      result.complete = true;
      return result;
    } finally {
      // Exceptions remain failures, with an incomplete record for final reporting.
      write(`${phase}.json`, JSON.stringify(result, null, 2));
    }
  }
  async function perform(operation, { phase, expectRepair = false, repairFirst = false } = {}) {
    files.mkdirSync(directory, { recursive: true });
    switch (operation) {
      case 'prepare': pull(); break;
      case 'probe': return probe(phase);
      case 'capture': {
        write('final-dns.log', read('/etc/resolv.conf'));
        const state = run('systemctl', ['is-active', 'agent'], 5000);
        write('agent-state.txt', state.stdout);
        break;
      }
      case 'report': {
        try {
          if (run('systemctl', ['is-active', '--quiet', 'agent'], 5000).code !== 0) throw new Error('StepSecurity agent is not active');
          const results = PHASES.filter(item => files.existsSync(path.join(directory, `${item}.json`)))
            .map(item => JSON.parse(files.readFileSync(path.join(directory, `${item}.json`), 'utf8')));
          const required = evaluate(results, expectRepair, repairFirst);
          summary('\n## Network diagnostic result\n\n**Passed.**\n\n');
          if (expectRepair && !repairFirst && results.find(item => item.phase === 'hardened')?.findings.length) {
            summary('Original DNS failed; recovery verified. Original evidence is retained.\n\n');
          }
          summary(`Verified allowed connectivity and enforcement for: ${required.join(', ')}.\n`);
        } catch (error) {
          summary(`\n## Network diagnostic result\n\n**Failed:** ${error.message}\n`);
          throw error;
        }
        break;
      }
      default: throw new Error(`Invalid diagnostic operation: ${operation}`);
    }
  }
  return { perform };
}
module.exports = { createDiagnostics, evaluate, hasRejection, IMAGE, URLS };
