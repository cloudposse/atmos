import { integer } from './api.mjs';

// The legacy workflow still needs its required gates during the split rollout.
// Inspect only acceptance shards: this gate itself is necessarily in progress.
export async function checkShards(api, { runId, attempt, target, count },
  { polls = 20, interval = 15000, log = console.log } = {}) {
  runId = integer(runId);
  attempt = integer(attempt);
  count = integer(count);
  if (!['linux', 'macos', 'windows'].includes(target)) throw new Error(`Invalid acceptance target: ${target}`);
  const prefix = `Acceptance Tests (${target}, shard `;
  const expected = new Set(Array.from({ length: count }, (_, i) => `${prefix}${i + 1}/${count})`));
  const currentAttempt = async () => {
    const run = await api.run(runId);
    if (run.run_attempt !== attempt) throw new Error('Acceptance verification attempt has been superseded');
  };

  for (let poll = 1; poll <= polls; poll++) {
    let shards;
    try {
      await currentAttempt();
      // Include successful jobs from earlier attempts when only failures were
      // rerun, but never let an earlier success mask a newer failed shard.
      const jobs = await api.list(`/actions/runs/${runId}/jobs?filter=latest`, 'jobs');
      shards = jobs.filter(job => job.name.startsWith(prefix));
      await currentAttempt();
    } catch (error) {
      const transient = error.status >= 500 || error.status === 429 || error instanceof TypeError ||
        ['AbortError', 'TimeoutError'].includes(error.name);
      if (!transient || poll === polls) throw error;
      log(`Poll ${poll}/${polls}: GitHub job listing unavailable; retrying`);
      await api.sleep(interval);
      continue;
    }

    // Count alone is insufficient: duplicates or changed numbering can hide a
    // missing shard. A different matrix shape fails immediately.
    if (shards.length !== count || new Set(shards.map(job => job.name)).size !== count ||
      shards.some(job => !expected.has(job.name))) {
      throw new Error(`Expected exactly ${count} distinct ${target} shards; found: ${shards.map(job => job.name).join(', ')}`);
    }
    if (shards.every(job => job.status === 'completed' && job.conclusion)) {
      const failed = shards.filter(job => job.conclusion !== 'success');
      if (failed.length) throw new Error(`Acceptance shards did not succeed: ${failed.map(job => `${job.name}: ${job.conclusion}`).join(', ')}`);
      log(`All ${count} ${target} acceptance shards succeeded`);
      return;
    }
    log(`Poll ${poll}/${polls}: waiting for ${target} shard conclusions`);
    if (poll < polls) await api.sleep(interval);
  }
  throw new Error(`Acceptance shard results never settled for ${target}`);
}
