import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
import { setTimeout } from 'node:timers/promises';

// Open the Homebrew/homebrew-core version-bump PR for atmos directly through the
// GitHub REST API instead of `brew bump-formula-pr`. The brew path failed on
// every stable release in five distinct ways (brew not on PATH, no local tap,
// `brew tap homebrew/core` refused without --force, and finally `git commit`
// with no runner git identity), because bump-formula-pr is a developer command
// that assumes a working brew + git checkout. The contents API commits with the
// token's identity server-side, so none of those runner preconditions apply.
// See docs/fixes/2026-10-24-homebrew-bump-api-rewrite.md.

const GITHUB_API = 'https://api.github.com';
const API_VERSION = '2022-11-28';
const USER_AGENT = 'cloudposse-atmos-homebrew-bump';

// The source tarball `url` line (never the `head` line, which uses the `head`
// keyword, nor `livecheck`'s unquoted `url :stable`).
const SOURCE_URL_RE =
  /^(\s+url\s+")https:\/\/github\.com\/cloudposse\/atmos\/archive\/refs\/tags\/[^"]+(")\s*$/m;

// The bare top-level source `sha256` line. Bottle checksums read
// `sha256 cellar: ...` or `sha256 arm64_tahoe: ...`, so a quote immediately
// after `sha256 ` can only be the source line.
const SOURCE_SHA_RE = /^(\s+sha256\s+")[0-9a-f]{64}(")\s*$/m;

// Strip a leading `v` and require an exact release version so a malformed tag
// never produces a bogus tarball URL or PR title.
export function normalizeVersion(raw) {
  const version = String(raw ?? '').trim().replace(/^v/, '');
  if (!/^\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?$/.test(version)) {
    throw new Error(`Homebrew bump requires an exact release version, got "${raw}".`);
  }
  return version;
}

// Replace the source `url` and `sha256` in place, asserting each line appears
// exactly once. A format change (extra match or none) fails loudly rather than
// silently corrupting the formula.
export function editFormula(formula, { tarballUrl, sha256 }) {
  const replaceOnce = (content, regex, replacement, label) => {
    const occurrences = content.match(new RegExp(regex.source, 'gm')) ?? [];
    if (occurrences.length !== 1) {
      throw new Error(`Expected exactly one ${label} line, found ${occurrences.length}; formula format may have changed.`);
    }
    return content.replace(regex, replacement);
  };
  let updated = replaceOnce(formula, SOURCE_URL_RE, `$1${tarballUrl}$2`, 'source url');
  updated = replaceOnce(updated, SOURCE_SHA_RE, `$1${sha256}$2`, 'source sha256');
  return updated;
}

// A human-honest PR body that survives homebrew-core's check_template.rb: it
// embeds the live upstream template verbatim (so ≥75% of its checklist/heading
// lines match regardless of future template edits) and adds the required AI
// disclosure. Boxes are left in their template state; BrewTestBot verifies the
// build and a human maintainer reviews before merge.
export function buildPullRequestBody(template) {
  const disclosure = [
    '',
    '---',
    '',
    'AI/LLM disclosure: this version bump was prepared with the assistance of',
    'Claude Code. A human maintainer (@aknysh) reviews every bump and is',
    'responsible for its correctness before it is merged.',
    '',
  ].join('\n');
  return `${template.trimEnd()}\n${disclosure}`;
}

// Download the release source tarball and return its lowercase sha256, the same
// digest Homebrew records for the `url` line.
async function computeTarballSha256(tarballUrl, fetchImpl, sleep) {
  const bytes = await download(tarballUrl, fetchImpl, sleep);
  return createHash('sha256').update(bytes).digest('hex');
}

async function download(url, fetchImpl, sleep) {
  for (let attempt = 0; ; attempt++) {
    let delay = 5000 * (attempt + 1);
    try {
      const response = await fetchImpl(url, { signal: AbortSignal.timeout(60000) });
      if (response.ok) return Buffer.from(await response.arrayBuffer());
      const retryAfter = response.headers.get('retry-after');
      if (retryAfter) {
        const seconds = Number(retryAfter);
        delay = Number.isFinite(seconds) ? seconds * 1000 : Date.parse(retryAfter) - Date.now();
        delay = Math.max(5000, delay || 5000);
      }
      try {
        await response.body?.cancel();
      } catch {
        // Cleanup must not override the HTTP status or the server's backoff limit.
      }
      const error = new Error(`Request failed (${response.status}): ${url}`);
      error.permanent = (response.status < 500 && ![408, 429].includes(response.status)) || delay > 60000;
      throw error;
    } catch (error) {
      if (error.permanent || attempt === 2) throw error;
      await sleep(delay);
    }
  }
}

// Minimal authenticated JSON client for the GitHub REST API. Returns the parsed
// body plus the raw response so callers can special-case statuses (202, 404,
// 422). Transient failures (network errors, 408/429/5xx) are retried with
// backoff that honors Retry-After; non-retryable statuses are returned as-is so
// callers (ensureFork's 404 poll, upsertBranch's 422, createPullRequest's 422)
// keep seeing them.
function makeClient(token, fetchImpl, sleep) {
  return async function request(method, path, { body, accept } = {}) {
    const url = path.startsWith('http') ? path : `${GITHUB_API}${path}`;
    const headers = {
      Authorization: `Bearer ${token}`,
      Accept: accept ?? 'application/vnd.github+json',
      'X-GitHub-Api-Version': API_VERSION,
      'User-Agent': USER_AGENT,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    };
    const serialized = body ? JSON.stringify(body) : undefined;
    for (let attempt = 0; ; attempt++) {
      let delay = 5000 * (attempt + 1);
      try {
        const response = await fetchImpl(url, {
          method,
          headers,
          ...(serialized ? { body: serialized } : {}),
          signal: AbortSignal.timeout(60000),
        });
        const transient = response.status === 408 || response.status === 429 || response.status >= 500;
        if (transient && attempt < 2) {
          const retryAfter = response.headers.get('retry-after');
          if (retryAfter) {
            const seconds = Number(retryAfter);
            delay = Number.isFinite(seconds) ? seconds * 1000 : Date.parse(retryAfter) - Date.now();
            delay = Math.max(5000, delay || 5000);
          }
          if (delay <= 60000) {
            try {
              await response.body?.cancel();
            } catch {
              // Cleanup must not override the retry decision.
            }
            await sleep(delay);
            continue;
          }
        }
        const text = await response.text();
        let data = {};
        if (text) {
          // Error responses from gateways/proxies can be HTML or plain text, not
          // JSON; keep the raw text as the message so ensureOk still reports it.
          try {
            data = JSON.parse(text);
          } catch {
            data = { message: text };
          }
        }
        return { ok: response.ok, status: response.status, data };
      } catch (error) {
        if (attempt === 2) throw error;
        await sleep(delay);
      }
    }
  };
}

function ensureOk(result, context) {
  if (!result.ok) {
    throw new Error(`${context} failed (${result.status}): ${result.data?.message ?? 'unknown error'}`);
  }
  return result.data;
}

// Create the fork if it does not already exist, then wait until the API reports
// it ready. Forks persist across releases, so this is normally a quick no-op.
async function ensureFork(request, upstreamOwner, upstreamRepo, login, sleep) {
  await request('POST', `/repos/${upstreamOwner}/${upstreamRepo}/forks`);
  for (let attempt = 0; attempt < 10; attempt++) {
    const fork = await request('GET', `/repos/${login}/${upstreamRepo}`);
    if (fork.ok) return;
    await sleep(5000);
  }
  throw new Error(`Fork ${login}/${upstreamRepo} did not become available.`);
}

// Point the branch ref at the given sha, creating it or force-updating a leftover
// branch from a re-run.
async function upsertBranch(request, owner, repo, branch, sha) {
  const create = await request('POST', `/repos/${owner}/${repo}/git/refs`, {
    body: { ref: `refs/heads/${branch}`, sha },
  });
  if (create.ok) return;
  if (create.status === 422) {
    ensureOk(
      await request('PATCH', `/repos/${owner}/${repo}/git/refs/heads/${branch}`, { body: { sha, force: true } }),
      'Update branch ref',
    );
    return;
  }
  ensureOk(create, 'Create branch ref');
}

// Open the PR, or return the existing one if a re-run already opened it (422
// "A pull request already exists").
async function createPullRequest(request, { upstreamOwner, upstreamRepo, base, head, title, body }) {
  const created = await request('POST', `/repos/${upstreamOwner}/${upstreamRepo}/pulls`, {
    body: { title, head, base, body, maintainer_can_modify: true },
  });
  if (created.ok) return created.data;
  if (created.status === 422) {
    const existing = ensureOk(
      await request('GET', `/repos/${upstreamOwner}/${upstreamRepo}/pulls?head=${encodeURIComponent(head)}&state=open`),
      'List existing pull requests',
    );
    if (existing.length > 0) return existing[0];
  }
  return ensureOk(created, 'Create pull request');
}

export async function bumpHomebrewFormula({
  version,
  token,
  upstreamOwner = 'Homebrew',
  upstreamRepo = 'homebrew-core',
  formulaPath = 'Formula/a/atmos.rb',
  sourceRepo = 'cloudposse/atmos',
  fetchImpl = fetch,
  sleep = setTimeout,
  log = console.log,
}) {
  if (!token) throw new Error('Homebrew bump requires a GitHub token (GH_BOT_TOKEN).');
  version = normalizeVersion(version);

  const tag = `v${version}`;
  const tarballUrl = `https://github.com/${sourceRepo}/archive/refs/tags/${tag}.tar.gz`;
  const sha256 = await computeTarballSha256(tarballUrl, fetchImpl, sleep);
  log(`Source ${tarballUrl} sha256 ${sha256}`);

  const request = makeClient(token, fetchImpl, sleep);
  const { login } = ensureOk(await request('GET', '/user'), 'Resolve bot account');

  await ensureFork(request, upstreamOwner, upstreamRepo, login, sleep);

  const base = ensureOk(await request('GET', `/repos/${upstreamOwner}/${upstreamRepo}`), 'Read upstream repo').default_branch;

  // Fast-forward the fork's default branch to upstream so the formula we edit is
  // current and the PR merges cleanly.
  ensureOk(
    await request('POST', `/repos/${login}/${upstreamRepo}/merge-upstream`, { body: { branch: base } }),
    'Sync fork with upstream',
  );

  const headSha = ensureOk(
    await request('GET', `/repos/${login}/${upstreamRepo}/git/ref/heads/${base}`),
    'Read fork head',
  ).object.sha;

  const branch = `bump-atmos-${version}`;
  await upsertBranch(request, login, upstreamRepo, branch, headSha);

  const file = ensureOk(
    await request('GET', `/repos/${login}/${upstreamRepo}/contents/${formulaPath}?ref=${branch}`),
    'Read formula',
  );
  const current = Buffer.from(file.content, 'base64').toString('utf8');
  const updated = editFormula(current, { tarballUrl, sha256 });
  if (updated === current) {
    throw new Error(`Formula ${formulaPath} already references ${tag}; nothing to bump.`);
  }

  ensureOk(
    await request('PUT', `/repos/${login}/${upstreamRepo}/contents/${formulaPath}`, {
      body: {
        message: `atmos ${version}`,
        content: Buffer.from(updated).toString('base64'),
        sha: file.sha,
        branch,
      },
    }),
    'Commit formula bump',
  );

  const template = (
    await download(
      `https://raw.githubusercontent.com/${upstreamOwner}/${upstreamRepo}/${base}/.github/PULL_REQUEST_TEMPLATE.md`,
      fetchImpl,
      sleep,
    )
  ).toString('utf8');

  const pr = await createPullRequest(request, {
    upstreamOwner,
    upstreamRepo,
    base,
    head: `${login}:${branch}`,
    title: `atmos ${version}`,
    body: buildPullRequestBody(template),
  });
  log(`Homebrew bump PR: ${pr.html_url}`);
  return pr;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await bumpHomebrewFormula({ version: process.env.TAG, token: process.env.GH_BOT_TOKEN });
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
