// Inputs contain only release identity and checkpoint outcomes, never credentials
// or complete step contexts (token-producing steps have sensitive outputs).
const escape = (value) => String(value).replace(/[&<>"'|\r\n]/g, (char) => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  '|': '&#124;', '\r': ' ', '\n': ' ',
})[char]);
const code = (value) => `<code>${escape(value)}</code>`;
const link = (url, label) => `<a href="${escape(url)}">${escape(label)}</a>`;
const results = { success: '✅ Success', failure: '❌ Failed', cancelled: '⏹ Cancelled', skipped: '⏭ Skipped' };
const stages = {
  prepare: ['Preparation', 'Release plan validated and tag reserved.'],
  build: ['Build', 'Artifacts built, signed, attested, and saved.'],
  verify: ['Verification', 'Artifact identities, signatures, and provenance verified; draft notes prepared and sealed artifacts saved.'],
  publish: ['Publication', 'Immutable release published and verified.'],
  complete: ['Milestone completion', 'Stable milestone bookkeeping completed.'],
};

function render(env) {
  const stage = stages[env.RELEASE_STAGE];
  if (!stage) throw new Error('Unknown release summary stage');
  const checks = JSON.parse(env.RELEASE_CHECKS);
  const root = `${env.GITHUB_SERVER_URL}/${env.GITHUB_REPOSITORY}`;
  const run = `${root}/actions/runs/${env.GITHUB_RUN_ID}`;
  const status = results[env.RELEASE_STATUS] || 'Unknown';
  const rows = [
    ['Result', status],
    ['Mode', code(env.RELEASE_MODE)],
    ['Reserved tag', env.RELEASE_TAG ? code(env.RELEASE_TAG) : 'Not reserved'],
  ];
  if (env.RELEASE_VERSION) rows.push(['Requested version', code(env.RELEASE_VERSION)]);
  if (env.RELEASE_COMMIT) rows.push(['Source commit', link(`${root}/commit/${encodeURIComponent(env.RELEASE_COMMIT)}`, env.RELEASE_COMMIT)]);
  if (env.RELEASE_MILESTONE) rows.push(['Milestone', link(`${root}/milestone/${encodeURIComponent(env.RELEASE_MILESTONE)}`, `#${env.RELEASE_MILESTONE}`)]);
  rows.push(['Workflow', link(run, `Run ${env.GITHUB_RUN_ID}, attempt ${env.GITHUB_RUN_ATTEMPT}`)]);
  if (env.RELEASE_ARTIFACT) rows.push(['Artifacts', link(`${run}/artifacts/${encodeURIComponent(env.RELEASE_ARTIFACT)}`, 'Download artifacts')]);
  if (env.RELEASE_STAGE === 'publish' && env.RELEASE_STATUS === 'success' && env.RELEASE_TAG) {
    rows.push(['Release', link(`${root}/releases/tag/${encodeURIComponent(env.RELEASE_TAG)}`, env.RELEASE_TAG)]);
  }
  let output = `## Atmos ${code(env.RELEASE_TAG || env.RELEASE_VERSION || env.RELEASE_MODE)} — ${stage[0]}\n\n`;
  output += '| Release | Details |\n| --- | --- |\n' + rows.map(([label, value]) => `| ${label} | ${value} |`).join('\n');
  output += '\n\n| Checkpoint | Result |\n| --- | --- |\n';
  output += checks.map(({ name, result }) => `| ${escape(name)} | ${results[result] || 'Not run'} |`).join('\n');
  if (env.RELEASE_STATUS === 'success') {
    output += `\n\n${stage[1]}\n`;
  } else {
    const failed = checks.filter(({ result }) => result === 'failure').map(({ name }) => escape(name));
    output += `\n\n${failed.length ? `Failed checkpoint: **${failed.join(', ')}**.` : 'This stage did not complete.'} See the ${link(run, 'job logs and annotations')} for the error.\n`;
    if (checks.some(({ name, result }) => name === 'Credentials' && result === 'failure')) {
      output += '\nCheck the selected environment’s `BOT_GITHUB_APP_PRIVATE_KEY` secret and `BOT_GITHUB_APP_CLIENT_ID` (or `BOT_GITHUB_APP_ID`) variable. The calling workflow must use `secrets: inherit`.\n';
    }
    if (env.RELEASE_TAG) output += '\nRetry this run to retain its reserved tag. Inspect publication state before dispatching another release.\n';
  }
  if (env.RELEASE_MODE !== 'cut') output += '\nPrereleases leave the stable milestone open.\n';
  return output;
}

module.exports = { render };
