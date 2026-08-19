#!/usr/bin/env node
/**
 * Local smoke test for verify-sha-pinning.
 *
 * Tests both the drift check (positive/negative tag-vs-SHA cases against the
 * real GitHub API, including forensic metadata) and the coverage check
 * (classifying unpinned, tag-pinned, and branch-pinned references).
 *
 * Usage: GITHUB_TOKEN=$(gh auth token) node .github/actions/verify-sha-pinning/test.mjs
 */

import fs from 'fs';
import path from 'path';
import os from 'os';

const token = process.env.GITHUB_TOKEN;
if (!token) {
  console.error('Error: GITHUB_TOKEN is required. Run with: GITHUB_TOKEN=$(gh auth token) node test.mjs');
  process.exit(1);
}

const headers = { Authorization: `Bearer ${token}`, Accept: 'application/vnd.github+json' };

const pattern = /uses:\s*([^\/\s]+)\/([^@\/\s]+)(?:\/[^@\s]+)?@([a-f0-9]{40})\s*#\s*(v\S+)/g;

// Coverage classifier — mirrors action.yml's parseUsesLine. Kept as a separate,
// deliberately duplicated implementation (same convention as the drift-check
// logic above) so this script exercises the real API without the Actions runtime.
function classifyUsesLine(line) {
  const m = line.match(/uses:\s*(\S+)(?:\s*#\s*(\S+))?/);
  if (!m) return null;
  const [, refValue, comment] = m;
  if (refValue.startsWith('./') || refValue.startsWith('../')) return null;

  const atIdx = refValue.lastIndexOf('@');
  if (atIdx === -1) return null;
  const refPath = refValue.slice(0, atIdx);
  const ref = refValue.slice(atIdx + 1);
  const parts = refPath.split('/');
  if (parts.length < 2) return null;
  const [owner, repo] = parts;

  if (/^[a-f0-9]{40}$/.test(ref)) {
    if (comment && /^v?\d/.test(comment)) {
      return { kind: 'tag-pinned', owner, repo, sha: ref, tag: comment };
    }
    return { kind: 'branch-pinned', owner, repo, sha: ref, ref: comment || null };
  }

  return { kind: 'unpinned', owner, repo, ref };
}

function scanCoverage(content) {
  const lines = content.split('\n');
  const refs = [];
  for (let i = 0; i < lines.length; i++) {
    const parsed = classifyUsesLine(lines[i]);
    if (!parsed) continue;
    refs.push({ line: i + 1, ...parsed });
  }
  return refs;
}

async function resolveTagSha(owner, repo, tag) {
  const res = await fetch(`https://api.github.com/repos/${owner}/${repo}/git/ref/tags/${tag}`, { headers });
  if (res.status === 404) throw new Error(`Tag "${tag}" not found in ${owner}/${repo}`);
  if (!res.ok) throw new Error(`API error ${res.status}: ${await res.text()}`);
  const ref = await res.json();

  if (ref.object.type === 'commit') return ref.object.sha;
  if (ref.object.type === 'tag') {
    const tagRes = await fetch(`https://api.github.com/repos/${owner}/${repo}/git/tags/${ref.object.sha}`, { headers });
    const tagObj = await tagRes.json();
    return tagObj.object.sha;
  }
  throw new Error(`Unexpected ref type: ${ref.object.type}`);
}

async function investigateMismatch(owner, repo, pinnedSha) {
  const details = { existsInRepo: false, matchingTags: [] };

  // Check if the pinned SHA exists in this repo
  try {
    const res = await fetch(`https://api.github.com/repos/${owner}/${repo}/commits/${pinnedSha}`, { headers });
    details.existsInRepo = res.ok;
  } catch {
    details.existsInRepo = false;
  }

  // Find which tags point to this SHA
  if (details.existsInRepo) {
    try {
      const res = await fetch(`https://api.github.com/repos/${owner}/${repo}/tags?per_page=100`, { headers });
      if (res.ok) {
        const tags = await res.json();
        details.matchingTags = tags.filter(t => t.commit.sha === pinnedSha).map(t => t.name);
      }
    } catch {
      // Non-fatal
    }
  }

  return details;
}

async function verify(content) {
  const lines = content.split('\n');
  const results = [];

  for (let i = 0; i < lines.length; i++) {
    let match;
    pattern.lastIndex = 0;
    while ((match = pattern.exec(lines[i])) !== null) {
      const [, owner, repo, pinnedSha, tag] = match;
      const label = `${owner}/${repo}@${tag}`;
      try {
        const resolved = await resolveTagSha(owner, repo, tag);
        const ok = resolved === pinnedSha;
        const result = { label, line: i + 1, ok, pinnedSha, resolved, owner, repo };

        if (!ok) {
          result.forensics = await investigateMismatch(owner, repo, pinnedSha);
        }

        results.push(result);
      } catch (err) {
        results.push({ label, line: i + 1, ok: false, pinnedSha, owner, repo, error: err.message });
      }
    }
  }
  return results;
}

function formatForensics(r) {
  if (!r.forensics) return '';
  if (!r.forensics.existsInRepo) {
    return `     ⚠️  Pinned SHA does not exist in ${r.owner}/${r.repo} — possible fork or typosquat`;
  }
  if (r.forensics.matchingTags.length > 0) {
    return `     ℹ️  Pinned SHA corresponds to: ${r.forensics.matchingTags.join(', ')}`;
  }
  return `     ℹ️  Pinned SHA exists in repo but has no matching tags`;
}

// unverifiable.json entry validation — mirrors action.yml's
// validateUnverifiableEntry (same deliberate-duplication convention as above).
function isNonEmptyString(value) {
  return typeof value === 'string' && value.trim().length > 0;
}

function validateUnverifiableEntry(entry, seen) {
  if (!isNonEmptyString(entry?.action)) {
    throw new Error(`Invalid unverifiable.json entry: missing or empty "action": ${JSON.stringify(entry)}`);
  }
  if (!isNonEmptyString(entry.reason)) {
    throw new Error(`Invalid unverifiable.json entry for "${entry.action}": missing or empty "reason"`);
  }
  if (!Array.isArray(entry.references) || entry.references.length === 0 || !entry.references.every(isNonEmptyString)) {
    throw new Error(`Invalid unverifiable.json entry for "${entry.action}": "references" must be a non-empty array of non-empty strings`);
  }
  if (seen.has(entry.action)) {
    throw new Error(`Invalid unverifiable.json: duplicate entry for "${entry.action}"`);
  }
}

// Downgrade eligibility — mirrors action.yml's outer catch: only a listed
// repo AND the explicitly verified access-block condition (HTTP 403) may
// ever be downgraded from a hard failure to 'unverifiable'.
function shouldDowngrade(err, hasException) {
  return Boolean(hasException) && err?.status === 403;
}

// ── Test cases ──────────────────────────────────────────────────

let passed = 0;
let failed = 0;

function assert(condition, name) {
  if (condition) {
    console.log(`  ✅ ${name}`);
    passed++;
  } else {
    console.error(`  ❌ ${name}`);
    failed++;
  }
}

// Test 1: Scan real workflow files (report results, don't fail test suite on real mismatches)
console.log('\n🧪 Test 1: Scan real workflow files');
const realWorkflow = fs.readFileSync('.github/workflows/atmos-pro.yaml', 'utf8');
const realResults = await verify(realWorkflow);
assert(realResults.length > 0, `Found ${realResults.length} SHA-pinned action(s)`);
for (const r of realResults) {
  if (r.ok) {
    console.log(`  ✅ ${r.label} — SHA matches`);
  } else {
    console.log(`  ⚠️  ${r.label} — SHA MISMATCH (real finding, not a test failure)`);
    if (r.resolved) {
      console.log(`     Pinned:   ${r.pinnedSha}`);
      console.log(`     Current:  ${r.resolved}`);
    }
    if (r.error) console.log(`     Error: ${r.error}`);
    const forensic = formatForensics(r);
    if (forensic) console.log(forensic);
  }
}

// Test 2: Deliberately wrong SHA (should fail, SHA won't exist in repo)
console.log('\n🧪 Test 2: Bad SHA (should be caught)');
const badWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v4.2.2
`;
const badResults = await verify(badWorkflow);
assert(badResults.length === 1, 'Found 1 SHA-pinned action');
assert(!badResults[0].ok, `Mismatch detected for ${badResults[0].label}`);
assert(badResults[0].forensics?.existsInRepo === false, 'Forensics: SHA does not exist in repo');
if (badResults[0].resolved) {
  console.log(`    Pinned:   ${badResults[0].pinnedSha}`);
  console.log(`    Expected: ${badResults[0].resolved}`);
  console.log(formatForensics(badResults[0]));
}

// Test 3: Wrong tag for valid SHA (tag doesn't match commit)
console.log('\n🧪 Test 3: Wrong tag comment (SHA belongs to different version)');
const wrongTagWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v6.0.2
`;
const wrongTagResults = await verify(wrongTagWorkflow);
assert(wrongTagResults.length === 1, 'Found 1 SHA-pinned action');
assert(!wrongTagResults[0].ok, `Mismatch detected — v4.2.2 SHA labeled as v6.0.2`);
assert(wrongTagResults[0].forensics?.existsInRepo === true, 'Forensics: SHA exists in repo');
assert(wrongTagResults[0].forensics?.matchingTags?.includes('v4.2.2'), 'Forensics: identifies actual tag as v4.2.2');
console.log(formatForensics(wrongTagResults[0]));

// Test 4: Annotated tag handling (bobheadxi/deployments uses annotated tags)
console.log('\n🧪 Test 4: Annotated tag dereference');
const annotatedWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: bobheadxi/deployments@648679e8e4915b27893bd7dbc35cb504dc915bc8 # v1
`;
const annotatedResults = await verify(annotatedWorkflow);
assert(annotatedResults.length === 1, 'Found 1 SHA-pinned action');
assert(annotatedResults[0].ok, `Annotated tag resolved correctly`);

// Test 5: Non-existent tag
console.log('\n🧪 Test 5: Non-existent tag (should fail)');
const noTagWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v99.99.99
`;
const noTagResults = await verify(noTagWorkflow);
assert(noTagResults.length === 1, 'Found 1 SHA-pinned action');
assert(!noTagResults[0].ok, `Missing tag detected`);
assert(noTagResults[0].error?.includes('not found'), `Error mentions tag not found`);

// Test 6: Lines without SHA pins are skipped
console.log('\n🧪 Test 6: Non-pinned actions are skipped');
const nonPinnedWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: ./.github/actions/verify-sha-pinning
`;
const nonPinnedResults = await verify(nonPinnedWorkflow);
assert(nonPinnedResults.length === 0, 'No SHA-pinned actions found (correct)');

// Test 7: Forensics populate correctly on mismatch
console.log('\n🧪 Test 7: Forensic metadata on mismatch');
// Reuse wrongTagResults from Test 3
const forensics = wrongTagResults[0].forensics;
assert(forensics !== undefined, 'Forensics object is populated');
assert(forensics?.existsInRepo === true, 'existsInRepo is true (SHA is valid in actions/checkout)');
assert(forensics?.matchingTags?.length > 0, `matchingTags has entries: [${forensics?.matchingTags?.join(', ')}]`);
assert(forensics?.matchingTags?.[0] === 'v4.2.2', `First matching tag is v4.2.2`);

// Test 8: Coverage check — bare tag (no SHA) is flagged unpinned
console.log('\n🧪 Test 8: Coverage — bare tag flagged as unpinned');
const bareTagWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: ./.github/actions/verify-sha-pinning
`;
const bareTagRefs = scanCoverage(bareTagWorkflow);
assert(bareTagRefs.length === 1, 'Local ref (./...) is skipped, only the bare tag is classified');
assert(bareTagRefs[0]?.kind === 'unpinned', 'actions/checkout@v6 classified as unpinned');
assert(bareTagRefs[0]?.owner === 'actions' && bareTagRefs[0]?.repo === 'checkout', 'owner/repo parsed correctly');

// Test 9: Coverage — branch-pinned SHA (# main) is covered but not tag-verified
console.log('\n🧪 Test 9: Coverage — branch-pinned ref classified separately from tag-pinned');
const branchPinWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: hashicorp/setup-packer@ce93c3c08a6c2ff2275bf4b54ff0d9a75f6c9789 # main
`;
const branchPinRefs = scanCoverage(branchPinWorkflow);
assert(branchPinRefs.length === 1, 'Found 1 reference');
assert(branchPinRefs[0]?.kind === 'branch-pinned', 'Classified as branch-pinned, not tag-pinned or unpinned');
assert(branchPinRefs[0]?.ref === 'main', 'Branch name captured from comment');

// Test 10: Coverage — tag comment without a leading "v" is still tag-like
console.log('\n🧪 Test 10: Coverage — no-"v"-prefix tag comment classified as tag-pinned');
const noVPrefixWorkflow = `
name: test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: cloudposse/github-action-seek-deployment@1234567890abcdef1234567890abcdef12345678 # 0.1.1
`;
const noVPrefixRefs = scanCoverage(noVPrefixWorkflow);
assert(noVPrefixRefs.length === 1, 'Found 1 reference');
assert(noVPrefixRefs[0]?.kind === 'tag-pinned', 'Classified as tag-pinned despite missing "v" prefix');
assert(noVPrefixRefs[0]?.tag === '0.1.1', 'Tag captured as "0.1.1"');

// Test 11: Coverage regression — real workflow directory has zero unpinned refs
console.log('\n🧪 Test 11: Coverage regression over real workflow files');
const workflowFiles = fs.readdirSync('.github/workflows')
  .filter(f => f.endsWith('.yml') || f.endsWith('.yaml'));
let realUnpinned = [];
for (const file of workflowFiles) {
  const content = fs.readFileSync(path.join('.github/workflows', file), 'utf8');
  for (const ref of scanCoverage(content)) {
    if (ref.kind === 'unpinned') {
      realUnpinned.push(`${file}:${ref.line} ${ref.owner}/${ref.repo}@${ref.ref}`);
    }
  }
}
if (realUnpinned.length > 0) {
  console.log('  Unpinned references found:');
  for (const u of realUnpinned) console.log(`    - ${u}`);
}
assert(realUnpinned.length === 0, `No unpinned third-party action references remain (found ${realUnpinned.length})`);

// Test 12: unverifiable.json — malformed entries are rejected
console.log('\n🧪 Test 12: unverifiable.json — malformed entries are rejected');
function throwsWith(fn, pattern) {
  try {
    fn();
    return false;
  } catch (err) {
    return pattern.test(err.message);
  }
}
assert(
  throwsWith(() => validateUnverifiableEntry({ action: 'foo/bar', references: ['https://example.com'] }, new Map()), /missing or empty "reason"/),
  'Entry missing "reason" is rejected'
);
assert(
  throwsWith(() => validateUnverifiableEntry({ action: 'foo/bar', reason: 'because' }, new Map()), /"references" must be/),
  'Entry missing "references" is rejected'
);
assert(
  throwsWith(() => validateUnverifiableEntry({ action: 'foo/bar', reason: 'because', references: [] }, new Map()), /"references" must be/),
  'Entry with empty "references" array is rejected'
);
assert(
  throwsWith(() => validateUnverifiableEntry({ reason: 'because', references: ['https://example.com'] }, new Map()), /missing or empty "action"/),
  'Entry missing "action" is rejected'
);

// Test 13: unverifiable.json — duplicate action entries are rejected
console.log('\n🧪 Test 13: unverifiable.json — duplicate action entries are rejected');
const seenDup = new Map();
const validEntry = { action: 'foo/bar', reason: 'because', references: ['https://example.com'] };
validateUnverifiableEntry(validEntry, seenDup);
seenDup.set(validEntry.action, validEntry);
assert(
  throwsWith(() => validateUnverifiableEntry({ action: 'foo/bar', reason: 'a different reason', references: ['https://example.com'] }, seenDup), /duplicate entry/),
  'Duplicate "action" key is rejected'
);

// Test 14: exception downgrade — only a 403 (access-blocked) error is eligible
console.log('\n🧪 Test 14: exception downgrade — only a 403 (access-blocked) error is eligible');
assert(shouldDowngrade({ status: 403 }, true) === true, '403 + listed repo → downgrade eligible');
assert(shouldDowngrade({ status: 404 }, true) === false, '404 (tag not found) + listed repo → NOT downgrade eligible, stays a failure');
assert(shouldDowngrade(new Error('boom'), true) === false, 'status-less error + listed repo → NOT downgrade eligible');
assert(shouldDowngrade({ status: 403 }, false) === false, '403 + unlisted repo → NOT downgrade eligible');

// Test 15: regression — a listed repository with a nonexistent tag must not
// be downgrade-eligible (only the documented 403 access-block condition is).
console.log('\n🧪 Test 15: regression — listed repo with a nonexistent tag stays a failure');
const bogusTagRes = await fetch('https://api.github.com/repos/aquasecurity/trivy-action/git/ref/tags/v0.0.0-does-not-exist', { headers });
if (bogusTagRes.status === 403) {
  console.log('  ⚠️  Skipped: this environment is itself IP-blocked by the aquasecurity org allow-list (403), so a missing tag can\'t be distinguished from the access-block here. Re-run from a non-blocked IP (e.g. not a GitHub-hosted Actions runner) to exercise this case.');
} else {
  assert(bogusTagRes.status !== 403, `Nonexistent-tag lookup returned ${bogusTagRes.status}, not 403`);
  assert(
    shouldDowngrade({ status: bogusTagRes.status }, true) === false,
    'Nonexistent tag on a listed repo is not downgrade-eligible — would remain failed_count=1, status=fail'
  );
}

// ── Summary ─────────────────────────────────────────────────────
console.log(`\n${'─'.repeat(50)}`);
console.log(`Results: ${passed} passed, ${failed} failed`);
process.exit(failed > 0 ? 1 : 0);
