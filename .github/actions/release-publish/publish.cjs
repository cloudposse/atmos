// The only path that publishes an Atmos release. Called from github-script;
// also tested with an in-memory GitHub client (no live releases in tests).
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');

const tagPattern = /^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
const commitPattern = /^[0-9a-f]{40}$/;
const sha256 = data => crypto.createHash('sha256').update(data).digest('hex');

function assertIdentity(release, identity) {
  if (!release.draft || release.id !== identity.id || release.tag_name !== identity.tag ||
      release.target_commitish !== identity.commit || release.prerelease !== identity.tag.includes('-')) {
    throw new Error(`Release identity changed: expected draft ${identity.id}, ${identity.tag} at ${identity.commit}`);
  }
}

function assertAssets(assets, expected) {
  if (assets.length !== Object.keys(expected).length || new Set(assets.map(a => a.name)).size !== assets.length) {
    throw new Error('Release asset set differs from the verified build');
  }
  for (const asset of assets) {
    if (!Object.hasOwn(expected, asset.name) || asset.state !== 'uploaded' ||
        asset.digest !== `sha256:${expected[asset.name]}`) {
      throw new Error(`Release asset mismatch: ${asset.name} (${asset.digest})`);
    }
  }
}

async function tagCommit(github, repo, tag) {
  let { data: { object } } = await github.rest.git.getRef({ ...repo, ref: `tags/${tag}` });
  // Peel annotated tags; compare commits, not annotated-tag object IDs.
  for (let depth = 0; object.type === 'tag' && depth < 8; depth++) {
    ({ data: { object } } = await github.rest.git.getTag({ ...repo, tag_sha: object.sha }));
  }
  if (object.type !== 'commit') throw new Error(`Tag ${tag} does not resolve to a commit`);
  return object.sha;
}

async function assertTag(github, repo, identity) {
  const actual = await tagCommit(github, repo, identity.tag);
  if (actual !== identity.commit) {
    throw new Error(`Tag commit mismatch: ${identity.tag} expected ${identity.commit}, got ${actual}`);
  }
}

async function prepare({ github, context, core }, releaseID, commit) {
  if (!/^[1-9]\d*$/.test(releaseID) || !commitPattern.test(commit)) {
    throw new Error('An existing draft release ID and a full lowercase commit SHA are required');
  }
  const repo = context.repo;
  const { data: release } = await github.rest.repos.getRelease({ ...repo, release_id: Number(releaseID) });
  if (!release.draft || !tagPattern.test(release.tag_name)) throw new Error('Expected a draft with a full v-prefixed release tag');
  const assets = await github.paginate(github.rest.repos.listReleaseAssets, { ...repo, release_id: release.id, per_page: 100 });
  if (assets.length) throw new Error('Draft already has assets. Refusing to reuse or overwrite an earlier build; use a fresh empty draft.');
  const { data: source } = await github.rest.repos.getCommit({ ...repo, ref: commit });
  if (source.sha !== commit) throw new Error('Source commit did not resolve exactly');
  const identity = { id: release.id, tag: release.tag_name, commit };
  try {
    await assertTag(github, repo, identity);
  } catch (error) {
    if (error.status !== 404) throw error;
    await github.rest.git.createRef({ ...repo, ref: `refs/tags/${identity.tag}`, sha: commit });
    await assertTag(github, repo, identity);
  }
  // Never leave main (or a feature branch) as the draft's eventual tag target.
  await github.rest.repos.updateRelease({ ...repo, release_id: identity.id, target_commitish: commit });
  core.setOutput('id', String(identity.id));
  core.setOutput('tag', identity.tag);
  core.setOutput('commit', identity.commit);
  return identity;
}

async function readManifest(dir, identity) {
  const bytes = await fs.readFile(path.join(dir, 'release-manifest.json'));
  const manifest = JSON.parse(bytes);
  if (manifest.tag !== identity.tag || manifest.commit !== identity.commit || !Object.keys(manifest.assets).length) {
    throw new Error('Verified manifest does not match release identity');
  }
  for (const [name, digest] of Object.entries(manifest.assets)) {
    if (!/^[A-Za-z0-9_.+-]+$/.test(name) || name === '.' || name === '..' || !/^[0-9a-f]{64}$/.test(digest)) {
      throw new Error(`Invalid manifest asset ${name}`);
    }
  }
  return { ...manifest.assets, 'release-manifest.json': sha256(bytes) };
}

async function publish({ github, context, core }, identity, dir) {
  const repo = context.repo;
  const expected = await readManifest(dir, identity);
  const getRelease = async () => (await github.rest.repos.getRelease({ ...repo, release_id: identity.id })).data;
  const listAssets = () => github.paginate(github.rest.repos.listReleaseAssets, { ...repo, release_id: identity.id, per_page: 100 });
  const release = await getRelease();
  assertIdentity(release, identity);
  await assertTag(github, repo, identity);
  const existing = await listAssets();
  const names = new Set();
  for (const asset of existing) {
    if (names.has(asset.name) || !Object.hasOwn(expected, asset.name) ||
        asset.state !== 'uploaded' || asset.digest !== `sha256:${expected[asset.name]}`) {
      throw new Error(`Existing release asset conflicts with verified build: ${asset.name}`);
    }
    names.add(asset.name);
  }

  for (const [name, digest] of Object.entries(expected)) {
    const data = await fs.readFile(path.join(dir, name));
    if (sha256(data) !== digest) throw new Error(`Local asset changed after verification: ${name}`);
    if (names.has(name)) continue; // Retry an identical partial upload without replacing assets.
    await github.rest.repos.uploadReleaseAsset({
      ...repo, release_id: identity.id, name, data,
      headers: { 'content-type': 'application/octet-stream', 'content-length': data.length },
    });
  }

  // This is the publication boundary: missing files, stale binaries, concurrent
  // edits and partial uploads all leave the release as a draft.
  assertAssets(await listAssets(), expected);
  assertIdentity(await getRelease(), identity);
  await assertTag(github, repo, identity);
  const { data: published } = await github.rest.repos.updateRelease({ ...repo, release_id: identity.id, draft: false, make_latest: identity.tag.includes('-') ? 'false' : 'true' });
  if (!published.immutable) throw new Error('Repository immutability was disabled; downstream distribution will reject this release');
  core.info(`Published ${identity.tag} at ${identity.commit}; all ${Object.keys(expected).length} assets verified`);
}

// Downstream workflows never distribute a manually published/unverified release.
async function verifyPublished({ github, context }, releaseID) {
  const repo = context.repo;
  const { data: release } = await github.rest.repos.getRelease({ ...repo, release_id: Number(releaseID) });
  if (release.draft || !release.immutable) throw new Error('Expected a published immutable release');
  const assets = await github.paginate(github.rest.repos.listReleaseAssets, { ...repo, release_id: release.id, per_page: 100 });
  const record = assets.find(asset => asset.name === 'release-manifest.json');
  if (!record) throw new Error('Release has no verified artifact manifest');
  const response = await github.rest.repos.getReleaseAsset({ ...repo, asset_id: record.id, headers: { accept: 'application/octet-stream' } });
  const bytes = Buffer.from(response.data);
  const manifest = JSON.parse(bytes.toString('utf8'));
  if (manifest.tag !== release.tag_name || !commitPattern.test(manifest.commit) ||
      release.target_commitish !== manifest.commit || release.prerelease !== manifest.tag.includes('-')) {
    throw new Error('Published release identity mismatch');
  }
  await assertTag(github, repo, { tag: manifest.tag, commit: manifest.commit });
  assertAssets(assets, { ...manifest.assets, 'release-manifest.json': sha256(bytes) });
}

// Atmos release-source predicate (https://atmos.tools/release/v1), not a SLSA
// predicate. GitHub's action generates standard SLSA provenance independently.
function provenance(context, identity, workflowSHA) {
  const repository = `https://github.com/${context.repo.owner}/${context.repo.repo}`;
  return {
    buildDefinition: {
      buildType: 'https://atmos.tools/release/v1',
      externalParameters: { releaseId: identity.id, tag: identity.tag, commit: identity.commit },
      resolvedDependencies: [{ uri: `git+${repository}@refs/tags/${identity.tag}`, digest: { gitCommit: identity.commit } }],
    },
    runDetails: {
      builder: { id: `${repository}/.github/workflows/release-pipeline.yml@${workflowSHA}` },
      metadata: { invocationId: `${repository}/actions/runs/${context.runId}` },
    },
  };
}

// The composite action only supplies its environment and GitHub client. Keep
// dispatch, input validation and summary behavior in the tested module.
async function run(api, env) {
  const releaseID = env.RELEASE_ID;
  const buildProvenance = env.RELEASE_OPERATION === 'provenance' && !releaseID;
  if (!buildProvenance && (!/^[1-9]\d*$/.test(releaseID || '') || !Number.isSafeInteger(Number(releaseID)))) {
    throw new Error('A positive numeric release ID is required');
  }
  const identity = { id: buildProvenance ? undefined : Number(releaseID), tag: env.RELEASE_TAG, commit: env.RELEASE_COMMIT };
  if (['publish', 'provenance'].includes(env.RELEASE_OPERATION) &&
      (!tagPattern.test(identity.tag || '') || !commitPattern.test(identity.commit || ''))) {
    throw new Error('A release tag and full source commit SHA are required');
  }
  switch (env.RELEASE_OPERATION) {
    case 'prepare':
      return prepare(api, releaseID, identity.commit);
    case 'publish':
      await publish(api, identity, env.RELEASE_DIRECTORY);
      await api.core.summary.addHeading('Verified release')
        .addTable([['Tag', 'Source commit', 'Release ID'],
          [identity.tag, identity.commit, String(identity.id)]])
        .write();
      return;
    case 'verify':
      return verifyPublished(api, identity.id);
    case 'provenance':
      await fs.writeFile(env.RELEASE_PROVENANCE,
        JSON.stringify(provenance(api.context, identity, env.RELEASE_WORKFLOW_SHA)));
      return;
    default:
      throw new Error(`Unknown release operation: ${env.RELEASE_OPERATION}`);
  }
}

module.exports = { assertIdentity, assertAssets, tagCommit, prepare, publish, verifyPublished, provenance, run };
