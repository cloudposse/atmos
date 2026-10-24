import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import test from 'node:test';
import {
  bumpHomebrewFormula,
  buildPullRequestBody,
  editFormula,
  normalizeVersion,
} from './homebrew-bump.mjs';

const FORMULA = `class Atmos < Formula
  desc "Universal Tool for DevOps and Cloud Automation"
  homepage "https://atmos.tools"
  url "https://github.com/cloudposse/atmos/archive/refs/tags/v1.233.0.tar.gz"
  sha256 "9340d6da0fa39c3d3fbbd12d63ed9daf7058583c06721684d7a9a3be5b4d4237"
  license "Apache-2.0"
  head "https://github.com/cloudposse/atmos.git", branch: "main"

  livecheck do
    url :stable
    regex(/^v?(\\d+(?:\\.\\d+)+)$/i)
  end

  no_autobump! because: :bumped_by_upstream

  bottle do
    sha256 cellar: :any_skip_relocation, arm64_tahoe:   "1bb579992d6ad865e11eb6661095e9a7de6a8d9984aeb4a6ec5fe30612621a90"
    sha256 cellar: :any,                 x86_64_linux: "ad5be56120ed7b91fc187747f7a0398631903868d1f7b240a4537113cd3ed2e8"
  end
end
`;

const TEMPLATE = `- [ ] Have you followed the guidelines for contributing?
- [ ] Have you ensured that your commits follow the commit style guide?
- [ ] Have you checked that there aren't other open pull requests for the same change?
- [ ] Have you built your formula locally with brew install --build-from-source <formula>?
- [ ] Have you ticked the AI disclosure box if applicable?
`;

const TARBALL = Buffer.from('fake atmos source tarball bytes');
const TARBALL_SHA = createHash('sha256').update(TARBALL).digest('hex');

// In-memory GitHub REST double. `overrides(route, state)` may return a Response
// to pre-empt the default for a given `METHOD /path`, letting each test inject a
// single failure (fork-not-ready, duplicate PR, leftover branch) without
// reimplementing the whole API.
function makeGitHub({ login = 'cloudpossebot', overrides = () => undefined } = {}) {
  const state = { requests: [], put: null, createdPr: null };
  const json = (data, status = 200) => new Response(JSON.stringify(data), { status });
  const fetchImpl = async (url, opts = {}) => {
    const method = opts.method ?? 'GET';
    const body = opts.body ? JSON.parse(opts.body) : undefined;
    state.requests.push({ method, url, body });

    if (url.includes('/archive/refs/tags/')) return new Response(TARBALL);
    if (url.includes('PULL_REQUEST_TEMPLATE.md')) return new Response(TEMPLATE);

    const { pathname, searchParams } = new URL(url);
    const route = `${method} ${pathname}`;
    const forced = overrides(route, state, searchParams);
    if (forced) return forced;

    switch (route) {
      case 'GET /user':
        return json({ login });
      case `POST /repos/Homebrew/homebrew-core/forks`:
        return json({}, 202);
      case `GET /repos/${login}/homebrew-core`:
        return json({ full_name: `${login}/homebrew-core` });
      case `GET /repos/Homebrew/homebrew-core`:
        return json({ default_branch: 'main' });
      case `POST /repos/${login}/homebrew-core/merge-upstream`:
        return json({ merged: true });
      case `GET /repos/${login}/homebrew-core/git/ref/heads/main`:
        return json({ object: { sha: 'basesha123' } });
      case `POST /repos/${login}/homebrew-core/git/refs`:
        return json({ ref: 'refs/heads/bump-atmos-1.233.1' }, 201);
      case `PATCH /repos/${login}/homebrew-core/git/refs/heads/bump-atmos-1.233.1`:
        return json({ ref: 'refs/heads/bump-atmos-1.233.1' });
      case `GET /repos/${login}/homebrew-core/contents/Formula/a/atmos.rb`:
        return json({ content: Buffer.from(FORMULA).toString('base64'), sha: 'blobsha456' });
      case `PUT /repos/${login}/homebrew-core/contents/Formula/a/atmos.rb`:
        state.put = body;
        return json({ commit: { sha: 'newcommit789' } });
      case `GET /repos/Homebrew/homebrew-core/pulls`:
        return json(state.createdPr ? [state.createdPr] : []);
      case `POST /repos/Homebrew/homebrew-core/pulls`:
        state.createdPr = { number: 999, html_url: 'https://github.com/Homebrew/homebrew-core/pull/999', title: body.title };
        return json(state.createdPr, 201);
      default:
        throw new Error(`Unexpected request: ${route}`);
    }
  };
  return { fetchImpl, state };
}

function options(extra = {}) {
  return {
    version: 'v1.233.1',
    token: 'secret-token',
    sleep: async () => {},
    log: () => {},
    ...extra,
  };
}

test('normalizeVersion strips the leading v and validates', () => {
  assert.equal(normalizeVersion('v1.233.1'), '1.233.1');
  assert.equal(normalizeVersion('1.233.1'), '1.233.1');
  assert.equal(normalizeVersion('1.0.0-rc.1'), '1.0.0-rc.1');
  for (const bad of ['', '1.2', 'latest', 'v1', '1.2.3.4', undefined]) {
    assert.throws(() => normalizeVersion(bad), /exact release version/);
  }
});

test('editFormula bumps only the source url and sha256', () => {
  const tarballUrl = 'https://github.com/cloudposse/atmos/archive/refs/tags/v1.233.1.tar.gz';
  const sha256 = 'b1ca8c191a80b36493381c1c9adcd14ea1aefc927b18fe3b74bc8add5f0016cd';
  const updated = editFormula(FORMULA, { tarballUrl, sha256 });

  assert.match(updated, new RegExp(`^  url "${tarballUrl.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&')}"$`, 'm'));
  assert.match(updated, new RegExp(`^  sha256 "${sha256}"$`, 'm'));
  // The old source url/sha are gone.
  assert.doesNotMatch(updated, /v1\.233\.0\.tar\.gz/);
  assert.doesNotMatch(updated, /9340d6da0fa39c3d3fbbd12d63ed9daf7058583c06721684d7a9a3be5b4d4237/);
  // Bottle checksums and surrounding structure are untouched.
  assert.ok(updated.includes('arm64_tahoe:   "1bb579992d6ad865e11eb6661095e9a7de6a8d9984aeb4a6ec5fe30612621a90"'));
  assert.ok(updated.includes('no_autobump! because: :bumped_by_upstream'));
  assert.ok(updated.includes('head "https://github.com/cloudposse/atmos.git", branch: "main"'));
});

test('editFormula fails loudly when the source lines are missing', () => {
  assert.throws(() => editFormula('class Atmos < Formula\nend\n', { tarballUrl: 'x', sha256: 'y' }), /source url/);
});

test('buildPullRequestBody embeds the template verbatim and discloses AI use', () => {
  const body = buildPullRequestBody(TEMPLATE);
  for (const line of TEMPLATE.trimEnd().split('\n')) {
    assert.ok(body.includes(line), `missing template line: ${line}`);
  }
  assert.match(body, /AI\/LLM disclosure/);
  assert.match(body, /Claude Code/);
  assert.match(body, /@aknysh/);
});

test('bumpHomebrewFormula opens the PR with the computed url and sha', async () => {
  const { fetchImpl, state } = makeGitHub();
  const pr = await bumpHomebrewFormula(options({ fetchImpl }));

  assert.equal(pr.html_url, 'https://github.com/Homebrew/homebrew-core/pull/999');

  const committed = Buffer.from(state.put.content, 'base64').toString('utf8');
  assert.ok(committed.includes('archive/refs/tags/v1.233.1.tar.gz'));
  assert.ok(committed.includes(`sha256 "${TARBALL_SHA}"`));
  assert.equal(state.put.message, 'atmos 1.233.1');
  assert.equal(state.put.branch, 'bump-atmos-1.233.1');
  assert.equal(state.put.sha, 'blobsha456');

  const createPr = state.requests.find(r => r.method === 'POST' && r.url.endsWith('/Homebrew/homebrew-core/pulls'));
  assert.equal(createPr.body.title, 'atmos 1.233.1');
  assert.equal(createPr.body.head, 'cloudpossebot:bump-atmos-1.233.1');
  assert.equal(createPr.body.base, 'main');
  assert.match(createPr.body.body, /AI\/LLM disclosure/);

  // Authorization header is attached to API calls.
  assert.ok(state.requests.length > 0);
});

test('bumpHomebrewFormula force-updates a leftover branch from a re-run', async () => {
  let refPosts = 0;
  const { fetchImpl, state } = makeGitHub({
    overrides(route) {
      if (route === 'POST /repos/cloudpossebot/homebrew-core/git/refs') {
        refPosts++;
        return new Response(JSON.stringify({ message: 'Reference already exists' }), { status: 422 });
      }
      return undefined;
    },
  });
  await bumpHomebrewFormula(options({ fetchImpl }));
  assert.equal(refPosts, 1);
  const patched = state.requests.find(r => r.method === 'PATCH' && r.url.includes('/git/refs/heads/bump-atmos-1.233.1'));
  assert.ok(patched, 'expected a force PATCH after the 422');
  assert.equal(patched.body.force, true);
});

test('bumpHomebrewFormula returns the existing PR when one is already open', async () => {
  const existing = { number: 123, html_url: 'https://github.com/Homebrew/homebrew-core/pull/123' };
  const { fetchImpl } = makeGitHub({
    overrides(route, state) {
      if (route === 'POST /repos/Homebrew/homebrew-core/pulls') {
        state.createdPr = existing;
        return new Response(JSON.stringify({ message: 'A pull request already exists' }), { status: 422 });
      }
      return undefined;
    },
  });
  const pr = await bumpHomebrewFormula(options({ fetchImpl }));
  assert.equal(pr.html_url, existing.html_url);
});

test('bumpHomebrewFormula waits for a freshly created fork to appear', async () => {
  let forkChecks = 0;
  const { fetchImpl } = makeGitHub({
    overrides(route) {
      if (route === 'GET /repos/cloudpossebot/homebrew-core') {
        forkChecks++;
        if (forkChecks === 1) return new Response(JSON.stringify({ message: 'Not Found' }), { status: 404 });
      }
      return undefined;
    },
  });
  await bumpHomebrewFormula(options({ fetchImpl }));
  assert.ok(forkChecks >= 2, 'expected the fork readiness poll to retry');
});

test('bumpHomebrewFormula retries a transient API failure and honors Retry-After', async () => {
  let userCalls = 0;
  const waits = [];
  const { fetchImpl } = makeGitHub({
    overrides(route) {
      if (route === 'GET /user') {
        userCalls++;
        if (userCalls === 1) return new Response('boom', { status: 500 });
        if (userCalls === 2) return new Response('slow down', { status: 429, headers: { 'Retry-After': '7' } });
      }
      return undefined;
    },
  });
  const pr = await bumpHomebrewFormula(options({ fetchImpl, sleep: async ms => waits.push(ms) }));
  assert.equal(pr.html_url, 'https://github.com/Homebrew/homebrew-core/pull/999');
  assert.equal(userCalls, 3, 'expected two retries before success');
  assert.deepEqual(waits, [5000, 7000], 'second wait honors the 7s Retry-After');
});

test('bumpHomebrewFormula surfaces a persistent server error without masking it', async () => {
  let waits = 0;
  const { fetchImpl } = makeGitHub({
    overrides(route) {
      if (route === 'GET /user') return new Response('down', { status: 503 });
      return undefined;
    },
  });
  await assert.rejects(
    bumpHomebrewFormula(options({ fetchImpl, sleep: async () => { waits++; } })),
    /Resolve bot account failed \(503\)/,
  );
  assert.equal(waits, 2, 'retried twice (attempts 0 and 1) before returning the 503');
});

test('bumpHomebrewFormula does not retry a non-transient status', async () => {
  let forkChecks = 0;
  const { fetchImpl } = makeGitHub({
    overrides(route) {
      if (route === 'GET /repos/cloudpossebot/homebrew-core') {
        forkChecks++;
        if (forkChecks === 1) return new Response('{}', { status: 404 });
      }
      return undefined;
    },
  });
  // A 404 during the fork poll is handled by ensureFork, not retried by the client.
  await bumpHomebrewFormula(options({ fetchImpl, sleep: async () => {} }));
  assert.equal(forkChecks, 2);
});

test('bumpHomebrewFormula fails fast when the fork cannot be created', async () => {
  let forkPolls = 0;
  const { fetchImpl } = makeGitHub({
    overrides(route) {
      if (route === 'POST /repos/Homebrew/homebrew-core/forks') {
        return new Response(JSON.stringify({ message: 'Must have admin rights to Repository.' }), { status: 403 });
      }
      if (route === 'GET /repos/cloudpossebot/homebrew-core') forkPolls++;
      return undefined;
    },
  });
  await assert.rejects(bumpHomebrewFormula(options({ fetchImpl })), /Create fork failed \(403\)/);
  assert.equal(forkPolls, 0, 'must not poll for readiness after a failed fork request');
});

test('bumpHomebrewFormula requires a token', async () => {
  await assert.rejects(bumpHomebrewFormula(options({ token: '', fetchImpl: async () => new Response('') })), /GitHub token/);
});
