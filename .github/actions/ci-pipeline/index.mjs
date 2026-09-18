import { appendFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { GitHub } from './api.mjs';
import { filename, policy, resolveRoot } from './source.mjs';
import { report } from './report.mjs';
import { coverageInputs, mergeCoverage } from './coverage.mjs';

const input = name => process.env[`INPUT_${name.toUpperCase()}`] ?? '';
function output(name, value) {
  const delimiter = randomUUID();
  appendFileSync(process.env.GITHUB_OUTPUT, `${name}<<${delimiter}\n${value ?? ''}\n${delimiter}\n`);
}

async function main() {
  const mode = input('mode');
  if (mode === 'merge-coverage') {
    mergeCoverage(input('coverage-directory'));
    return;
  }
  const api = new GitHub(process.env.GITHUB_REPOSITORY, input('github-token'));
  const run = await api.run(input('run-id'));
  if (mode === 'report') {
    const result = await report(api, run);
    appendFileSync(process.env.GITHUB_STEP_SUMMARY, `CI report: ${JSON.stringify(result)}\n`);
    return;
  }
  if (!['resolve', 'coverage'].includes(mode)) throw new Error(`Unknown mode: ${mode}`);
  const source = await resolveRoot(api, run);
  const expectedAttempt = input('attempt');
  const active = source.active && (!expectedAttempt || Number(expectedAttempt) === run.run_attempt);
  output('active', active);
  if (!active) return;
  if (mode === 'coverage') {
    for (const [key, value] of Object.entries(await coverageInputs(api, source))) output(key, value);
    return;
  }
  // Direct children may only consume the platform named by their trusted definition.
  const currentFile = process.env.GITHUB_WORKFLOW_REF?.split('@')[0].split('/').at(-1);
  if (policy[currentFile] && filename(source.root) !== policy[currentFile].parent) throw new Error('Unexpected build platform');
  for (const [key, value] of Object.entries({ revision: source.revision, base: source.base,
    event: source.event, draft: source.draft, 'run-id': source.root.id,
    'head-sha': source.root.head_sha, 'pr-number': source.pr?.number ?? '' })) output(key, value);
}

main().catch(error => {
  console.error(`::error::${String(error.message).replaceAll('%', '%25').replaceAll('\n', '%0A').replaceAll('\r', '%0D')}`);
  process.exitCode = 1;
});
