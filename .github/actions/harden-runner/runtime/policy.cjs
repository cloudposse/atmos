const fs = require('node:fs');
const path = require('node:path');

function readPolicy(content, name) {
  const policies = JSON.parse(content);
  if (!Object.hasOwn(policies, name) || !Array.isArray(policies[name]) || !policies[name].length) {
    throw new Error(`Missing or empty endpoint policy: ${name}`);
  }
  for (const endpoint of policies[name]) {
    if (typeof endpoint !== 'string' || !/^(?:\*\.)?[a-z0-9]+(?:[.-][a-z0-9]+)*:[0-9]+$/.test(endpoint)) {
      throw new Error(`Invalid endpoint in policy ${name}`);
    }
    const port = Number(endpoint.slice(endpoint.lastIndexOf(':') + 1));
    if (port < 1 || port > 65535) throw new Error(`Invalid port in policy ${name}`);
  }
  return [...new Set(policies[name])].join(' ');
}
function prepare({ files = fs, env = process.env } = {}) {
  const content = files.readFileSync(path.join(__dirname, '..', 'endpoints.json'), 'utf8');
  const endpoints = readPolicy(content, env.INPUT_POLICY);
  files.appendFileSync(env.GITHUB_ENV, `ATMOS_HARDEN_RUNNER_ENDPOINTS=${endpoints}\n`);
}
module.exports = { readPolicy, prepare };
