#!/usr/bin/env node
/**
 * Local smoke test for verify-sha-pinning.
 *
 * Tests both positive (valid pins) and negative (bad SHA) cases
 * against the real GitHub API, including forensic metadata.
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

// ── Summary ─────────────────────────────────────────────────────
console.log(`\n${'─'.repeat(50)}`);
console.log(`Results: ${passed} passed, ${failed} failed`);
process.exit(failed > 0 ? 1 : 0);
