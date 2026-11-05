// Test transport only. All publication decisions come from the production action.
// The Go rehearsal owns the mock GitHub state; no GitHub credentials are used.
const { publish, verifyPublished } = require('../../../.github/actions/release-publish/publish.cjs');

async function main() {
  const [endpoint, directory, tag, commit, operation = 'publish'] = process.argv.slice(2);
  const origin = new URL(endpoint);
  if (origin.hostname !== '127.0.0.1' || origin.protocol !== 'http:') {
    throw new Error('Rehearsal requires a local mock server');
  }
  async function call(method, args = {}) {
    const payload = { ...args };
    if (Buffer.isBuffer(payload.data)) payload.data = payload.data.toString('base64');
    const response = await fetch(`${origin.origin}/__publisher/${method}`, {
      method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(payload),
      redirect: 'error',
    });
    if (!response.ok) throw new Error(`Mock ${method} failed: ${response.status}`);
    return { data: await response.json() };
  }
  const repos = {};
  for (const method of ['getRelease', 'listReleaseAssets', 'updateRelease', 'uploadReleaseAsset']) {
    repos[method] = args => call(method, args);
  }
  repos.getReleaseAsset = async args => ({ data: Buffer.from((await call('getReleaseAsset', args)).data, 'base64') });
  const api = {
    github: {
      rest: { repos, git: { getRef: args => call('getRef', args) } },
      paginate: async (method, args) => (await method(args)).data,
    },
    context: { repo: { owner: 'cloudposse', repo: 'atmos' } },
    core: { info: () => {} },
  };
  if (operation === 'publish') await publish(api, { id: 99, tag, commit }, directory);
  await verifyPublished(api, 99);
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });
