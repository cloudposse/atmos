const fs = require('node:fs');
const path = require('node:path');
const { parse } = require('./yaml.cjs');

function readPolicy(content, selection) {
  const policies = parse(content);
  if (!policies || typeof policies !== 'object' || Array.isArray(policies)) {
    throw new Error('Allowlist must be a mapping of endpoint groups');
  }
  const names = typeof selection === 'string' ? selection.trim().split(/\s+/) : [];
  if (!names.length || !names[0]) throw new Error('Select at least one endpoint group');
  const endpoints = new Set();
  for (const name of names) {
    if (!Object.hasOwn(policies, name) || !Array.isArray(policies[name])) {
      throw new Error(`Missing or invalid endpoint group: ${name}`);
    }
    for (const endpoint of policies[name]) {
      if (typeof endpoint !== 'string' || !/^(?:\*\.)?[a-z0-9]+(?:[.-][a-z0-9]+)*:[0-9]+$/.test(endpoint)) {
        throw new Error(`Invalid endpoint in group ${name}`);
      }
      const port = Number(endpoint.slice(endpoint.lastIndexOf(':') + 1));
      if (port < 1 || port > 65535) throw new Error(`Invalid port in group ${name}`);
      endpoints.add(endpoint);
    }
  }
  return [...endpoints].join(' ');
}
function prepare({ files = fs, env = process.env } = {}) {
  // $/ downloads the repository at the action's SHA. Resolve from that archive,
  // never from the workspace: no checkout is required and later checkout cannot replace it.
  const content = files.readFileSync(path.join(__dirname, '..', '..', '..', 'allowlist.yaml'), 'utf8');
  const endpoints = readPolicy(content, env.INPUT_POLICIES);
  files.appendFileSync(env.GITHUB_ENV, `ATMOS_HARDEN_RUNNER_ENDPOINTS=${endpoints}\n`);
}
module.exports = { readPolicy, prepare };
