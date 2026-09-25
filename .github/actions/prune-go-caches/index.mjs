import { appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const gracePeriod = 24 * 60 * 60 * 1000;
// Restrict deletion to immutable generations written by our warmup. Match the
// complete OS/architecture/Go/dependency-hash lineage, never a broad prefix.
const generation = /^((?:go-cache|race-go-cache|race-plan-go-cache|intel-go-cache)-(?:Linux|Windows|macOS)-(?:X64|ARM64)-go[^-]+-[a-f0-9]{64})-\d+$/;

export function pruneCandidates(caches, now = Date.now()) {
  const lineages = new Map();
  for (const cache of caches) {
    const match = generation.exec(cache.key);
    if (cache.ref !== 'refs/heads/main' || !match || !Number.isFinite(Date.parse(cache.created_at))) continue;
    const entries = lineages.get(match[1]) ?? [];
    entries.push(cache);
    lineages.set(match[1], entries);
  }
  return [...lineages.values()].flatMap(entries => entries
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at) || b.id - a.id)
    .slice(2)
    .filter(cache => now - Date.parse(cache.created_at) >= gracePeriod));
}

export async function prune({ token, repository, apiUrl = 'https://api.github.com', dryRun = true, request = fetch }) {
  if (!token || !/^[\w.-]+\/[\w.-]+$/.test(repository ?? '')) throw new Error('Token and owner/repository are required');
  const headers = { authorization: `Bearer ${token}`, accept: 'application/vnd.github+json' };
  const base = `${apiUrl}/repos/${repository}/actions/caches`;
  const caches = [];
  // Collect every page before deleting: deleting while paginating shifts offsets.
  for (let page = 1; ; page++) {
    const response = await request(`${base}?per_page=100&page=${page}&ref=refs%2Fheads%2Fmain`, { headers });
    if (!response.ok) throw new Error(`Cache inventory failed: HTTP ${response.status}`);
    const body = await response.json();
    caches.push(...body.actions_caches);
    if (body.actions_caches.length < 100) break;
  }
  const candidates = pruneCandidates(caches);
  let reclaimed = 0;
  for (const cache of candidates) {
    console.log(`${dryRun ? 'Would delete' : 'Deleting'} ${cache.id}: ${cache.key} (${cache.size_in_bytes} bytes)`);
    if (!dryRun) {
      const response = await request(`${base}/${cache.id}`, { method: 'DELETE', headers });
      // Another cleanup/eviction may already have removed this immutable entry.
      if (!response.ok && response.status !== 404) throw new Error(`Cache deletion failed for ${cache.id}: HTTP ${response.status}`);
    }
    reclaimed += cache.size_in_bytes;
  }
  console.log(`${candidates.length} superseded caches; ${(reclaimed / 2 ** 30).toFixed(2)} GiB ${dryRun ? 'eligible' : 'reclaimed'}`);
  return reclaimed;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const bytes = await prune({
      token: process.env['INPUT_GITHUB-TOKEN'],
      repository: process.env.GITHUB_REPOSITORY,
      apiUrl: process.env.GITHUB_API_URL,
      dryRun: process.env['INPUT_DRY-RUN'] !== 'false',
    });
    if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `reclaimed-bytes=${bytes}\n`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
