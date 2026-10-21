const assert = require('node:assert/strict');
const { test } = require('node:test');
const { assertIdentity, assertAssets, prepare, publish, verifyPublished, tagCommit, provenance, run } = require('./publish.cjs');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');

const commit = '12f83b5248253662d8af06030e3966034a7ef5d4';
const identity = { id: 123, tag: 'v1.233.0', commit };
const release = { id: 123, tag_name: identity.tag, target_commitish: commit, draft: true };

test('publication accepts only the same draft, tag, and pinned commit', () => {
  assert.doesNotThrow(() => assertIdentity(release, identity));
  for (const change of [{ id: 456 }, { tag_name: 'v1.234.0' }, { target_commitish: 'main' }, { draft: false }]) {
    assert.throws(() => assertIdentity({ ...release, ...change }, identity), /identity changed/);
  }
});

test('uploaded assets must match the entire verified set, including digests', () => {
  const expected = { 'atmos_1.233.0_linux_amd64': 'a'.repeat(64) };
  const asset = { name: Object.keys(expected)[0], state: 'uploaded', digest: `sha256:${'a'.repeat(64)}` };
  assert.doesNotThrow(() => assertAssets([asset], expected));
  for (const assets of [[], [asset, { ...asset, name: 'stale' }], [{ ...asset, digest: `sha256:${'b'.repeat(64)}` }], [{ ...asset, digest: null }], [{ ...asset, state: 'new' }]]) {
    assert.throws(() => assertAssets(assets, expected), /asset/);
  }
});

const digest = data => `sha256:${crypto.createHash('sha256').update(data).digest('hex')}`;

test('provenance identifies the built commit separately from the workflow commit', () => {
  const context = { repo: { owner: 'cloudposse', repo: 'atmos' }, runId: 42 };
  const predicate = provenance(context, identity, 'a'.repeat(40));
  assert.equal(predicate.buildDefinition.resolvedDependencies[0].digest.gitCommit, commit);
  assert.match(predicate.runDetails.builder.id, /@a{40}$/);
});

function fakeGitHub(overrides = {}) {
  const state = { release: { ...release }, assets: [], tag: commit, immutable: true, published: 0, uploads: 0, ...overrides };
  const github = {
    request: async () => ({ data: { enabled: state.immutable } }),
    paginate: async () => state.assets.map(asset => ({ ...asset })),
    rest: {
      git: {
        getRef: async () => {
          if (!state.tag) throw Object.assign(new Error('not found'), { status: 404 });
          return { data: { object: { type: 'commit', sha: state.tag } } };
        },
        createRef: async ({ sha }) => { state.tag = sha; },
      },
      repos: {
        getRelease: async () => ({ data: { ...state.release } }),
        getCommit: async ({ ref }) => ({ data: { sha: ref } }),
        listReleaseAssets: () => {},
        updateRelease: async (args) => {
          if (args.draft === false) {
            state.published++;
            state.release.immutable = state.immutable;
          }
          Object.assign(state.release, args);
          return { data: state.release };
        },
        uploadReleaseAsset: async ({ name, data }) => {
          state.uploads++;
          if (state.failUpload === state.uploads) throw new Error('upload failed');
          state.assets.push({ id: state.uploads, name, data, state: 'uploaded', digest: digest(data) });
          if (state.afterUpload) state.afterUpload(state);
        },
        getReleaseAsset: async ({ asset_id }) => ({ data: state.assets.find(a => a.id === asset_id).data }),
      },
    },
  };
  const outputs = {};
  return {
    state, outputs, github, context: { repo: { owner: 'cloudposse', repo: 'atmos' } },
    core: { setOutput: (key, value) => { outputs[key] = value; }, info: () => {} },
  };
}

async function fixture(t) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'release-guard-'));
  t.after(() => fs.rm(dir, { recursive: true, force: true }));
  const binary = Buffer.from('verified fixture binary');
  const name = 'atmos_1.233.0_linux_amd64';
  await fs.writeFile(path.join(dir, name), binary);
  await fs.writeFile(path.join(dir, 'release-manifest.json'), JSON.stringify({
    tag: identity.tag, commit, assets: { [name]: digest(binary).slice(7) },
  }));
  return dir;
}

test('prepare pins a missing remote tag and draft target to the explicit commit', async () => {
  const api = fakeGitHub({ tag: null, release: { ...release, target_commitish: 'main' } });
  assert.deepEqual(await prepare(api, '123', commit), identity);
  assert.equal(api.state.tag, commit);
  assert.equal(api.state.release.target_commitish, commit);
  assert.deepEqual(api.outputs, { id: '123', tag: identity.tag, commit });
  assert.equal(api.state.published, 0);
});

test('prepare never changes an existing tag or reuses old draft assets', async () => {
  for (const state of [
    { tag: 'b392e54f74a9c436d2c27fc8ba1b82605e95b58e' },
    { assets: [{ name: 'atmos_1.234.0_linux_amd64' }] },
    { release: { ...release, draft: false } },
  ]) {
    const api = fakeGitHub(state);
    await assert.rejects(prepare(api, '123', commit));
    assert.equal(api.state.published, 0);
    assert.equal(api.state.uploads, 0);
  }
});

test('annotated tags are compared by their peeled commit', async () => {
  const api = fakeGitHub();
  api.github.rest.git.getRef = async () => ({ data: { object: { type: 'tag', sha: 'tag-object' } } });
  api.github.rest.git.getTag = async () => ({ data: { object: { type: 'commit', sha: commit } } });
  assert.equal(await tagCommit(api.github, api.context.repo, identity.tag), commit);
});

test('only a complete verified upload publishes, and downstream verifies the same manifest', async t => {
  const api = fakeGitHub();
  await publish(api, identity, await fixture(t));
  assert.equal(api.state.published, 1);
  assert.equal(api.state.assets.length, 2);
  await verifyPublished(api, 123);
});

test('all publication failure modes leave the release unpublished', async t => {
  const cases = {
    'partial upload': { failUpload: 2 },
    'changed draft tag': { afterUpload: state => { state.release.tag_name = 'v1.234.0'; } },
    'moved remote tag': { afterUpload: state => { state.tag = 'a'.repeat(40); } },
    'changed commit target': { afterUpload: state => { state.release.target_commitish = 'main'; } },
    'wrong uploaded bytes': { afterUpload: state => { state.assets[0].digest = `sha256:${'b'.repeat(64)}`; } },
    'unexpected uploaded asset': { afterUpload: state => { state.assets.push({ name: 'stale', digest: 'sha256:bad' }); } },
    'draft acquired old assets': { assets: [{ name: 'stale' }] },
  };
  for (const [name, state] of Object.entries(cases)) {
    await t.test(name, async t => {
      const api = fakeGitHub(state);
      await assert.rejects(publish(api, identity, await fixture(t)));
      assert.equal(api.state.published, 0);
      assert.equal(api.state.release.draft, true);
    });
  }
});

test('local changes after verification fail before publication', async t => {
  const dir = await fixture(t);
  await fs.writeFile(path.join(dir, 'atmos_1.233.0_linux_amd64'), 'changed');
  const api = fakeGitHub();
  await assert.rejects(publish(api, identity, dir), /Local asset changed/);
  assert.equal(api.state.published, 0);
});

test('downstream rejects manual publication without verified artifacts', async () => {
  for (const state of [
    { release: { ...release, draft: false, immutable: false } },
    { release: { ...release, draft: false, immutable: true } },
  ]) {
    await assert.rejects(verifyPublished(fakeGitHub(state), 123));
  }
});

test('action entrypoint prepares, records provenance, publishes and verifies the same release', async t => {
  const api = fakeGitHub({ tag: null });
  const dir = await fixture(t);
  const env = {
    RELEASE_ID: '123', RELEASE_TAG: identity.tag, RELEASE_COMMIT: commit,
    RELEASE_DIRECTORY: dir, RELEASE_PROVENANCE: path.join(dir, 'provenance.json'),
    RELEASE_WORKFLOW_SHA: 'a'.repeat(40),
  };
  await run(api, { ...env, RELEASE_OPERATION: 'prepare' });
  assert.deepEqual(api.outputs, { id: '123', tag: identity.tag, commit });
  await run(api, { ...env, RELEASE_OPERATION: 'provenance' });
  const predicate = JSON.parse(await fs.readFile(env.RELEASE_PROVENANCE));
  assert.equal(predicate.buildDefinition.resolvedDependencies[0].digest.gitCommit, commit);
  let table;
  let summaries = 0;
  api.core.summary = {
    addHeading() { return this; },
    addTable(value) { table = value; return this; },
    async write() { summaries++; },
  };
  await run(api, { ...env, RELEASE_OPERATION: 'publish' });
  assert.equal(api.state.published, 1);
  assert.equal(summaries, 1);
  assert.deepEqual(table[1], [identity.tag, commit, '123']);
  await run(api, { ...env, RELEASE_OPERATION: 'verify' });
});

test('action entrypoint rejects malformed inputs and never reports a failed publication as successful', async t => {
  const api = fakeGitHub({ failUpload: 1 });
  const env = {
    RELEASE_OPERATION: 'publish', RELEASE_ID: '123', RELEASE_TAG: identity.tag,
    RELEASE_COMMIT: commit, RELEASE_DIRECTORY: await fixture(t),
  };
  for (const change of [
    { RELEASE_ID: '' }, { RELEASE_ID: '12oops' }, { RELEASE_ID: '9007199254740993' },
    { RELEASE_TAG: 'latest' }, { RELEASE_COMMIT: 'main' }, { RELEASE_OPERATION: 'unknown' },
  ]) {
    await assert.rejects(run(api, { ...env, ...change }));
  }
  assert.equal(api.state.uploads, 0);
  let summaries = 0;
  api.core.summary = { addHeading() { summaries++; return this; } };
  await assert.rejects(run(api, env), /upload failed/);
  assert.equal(api.state.published, 0);
  assert.equal(summaries, 0);
});

test('preparation rejects malformed identities and a source that does not resolve exactly', async () => {
  const api = fakeGitHub();
  for (const [id, sha] of [['0', commit], ['abc', commit], ['123', 'main']]) {
    await assert.rejects(prepare(api, id, sha), /full lowercase commit SHA/);
  }
  await assert.rejects(prepare(fakeGitHub({ release: { ...release, tag_name: 'latest' } }), '123', commit), /full v-prefixed/);
  api.github.rest.repos.getCommit = async () => ({ data: { sha: 'b'.repeat(40) } });
  await assert.rejects(prepare(api, '123', commit), /did not resolve exactly/);
  assert.equal(api.state.published, 0);
  assert.deepEqual(api.outputs, {});
});

test('tag resolution rejects non-commit targets and bounds annotated-tag cycles', async () => {
  const api = fakeGitHub();
  api.github.rest.git.getRef = async () => ({ data: { object: { type: 'blob', sha: commit } } });
  await assert.rejects(tagCommit(api.github, api.context.repo, identity.tag), /does not resolve to a commit/);
  let lookups = 0;
  const cycle = { data: { object: { type: 'tag', sha: 'c'.repeat(40) } } };
  api.github.rest.git.getRef = async () => cycle;
  api.github.rest.git.getTag = async () => { lookups++; return cycle; };
  await assert.rejects(tagCommit(api.github, api.context.repo, identity.tag), /does not resolve to a commit/);
  assert.equal(lookups, 8);
});

test('malformed or mismatched manifests cannot trigger any upload', async t => {
  const dir = await fixture(t);
  const manifestPath = path.join(dir, 'release-manifest.json');
  const manifest = JSON.parse(await fs.readFile(manifestPath));
  const variants = [
    '{invalid json',
    { ...manifest, tag: 'v1.234.0' },
    { ...manifest, commit: 'b'.repeat(40) },
    { ...manifest, assets: {} },
    { ...manifest, assets: { '../secret': 'a'.repeat(64) } },
    { ...manifest, assets: { '..': 'a'.repeat(64) } },
    { ...manifest, assets: { 'asset': 'invalid digest' } },
  ];
  for (const variant of variants) {
    await fs.writeFile(manifestPath, typeof variant === 'string' ? variant : JSON.stringify(variant));
    const api = fakeGitHub();
    await assert.rejects(publish(api, identity, dir));
    assert.equal(api.state.uploads, 0);
    assert.equal(api.state.published, 0);
  }
  await fs.unlink(manifestPath);
  await assert.rejects(publish(fakeGitHub(), identity, dir), /ENOENT/);
});

test('disabling immutability outside the workflow is reported and blocks downstream distribution', async t => {
  const api = fakeGitHub({ immutable: false });
  await assert.rejects(publish(api, identity, await fixture(t)), /immutability was disabled/);
  // The privileged setting change is outside our publication boundary; the
  // API has published, but downstream must not silently accept that release.
  assert.equal(api.state.published, 1);
  await assert.rejects(verifyPublished(api, 123), /immutable release/);
});

test('downstream rejects a manifest naming a different release or invalid commit', async t => {
  for (const changes of [{ tag: 'v1.234.0' }, { commit: 'main' }]) {
    const api = fakeGitHub();
    await publish(api, identity, await fixture(t));
    const record = api.state.assets.find(a => a.name === 'release-manifest.json');
    record.data = Buffer.from(JSON.stringify({ ...JSON.parse(record.data), ...changes }));
    record.digest = digest(record.data);
    await assert.rejects(verifyPublished(api, 123), /identity mismatch/);
  }
});
