import { execFileSync } from 'node:child_process';
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { childRun, filename, peerRoot } from './source.mjs';
import { resultForJobs } from './report.mjs';

export async function coverageInputs(api, source) {
  const linux = filename(source.root) === 'ci-build-linux.yml' ? source.root : await peerRoot(api, 'ci-build-linux.yml', source);
  const go = filename(source.root) === 'ci-test-go.yml' ? source.root : await peerRoot(api, 'ci-test-go.yml', source);
  if (!linux || !go || linux.conclusion !== 'success') return { ready: false };
  const tests = await childRun(api, 'ci-test-linux.yml', linux);
  if (!tests) return { ready: false };
  const shards = Array.from({ length: 10 }, (_, i) => `Acceptance Tests (linux, shard ${i + 1}/10)`);
  if (resultForJobs(tests, await api.jobs(tests, shards), shards) !== 'success') return { ready: false };
  // Mage artifacts are available before the independent race shards complete.
  const mage = (await api.jobs(go)).find(job => job.name === '[magefiles] unit tests');
  if (mage?.conclusion !== 'success') return { ready: false };
  return { ready: true, 'test-run-id': tests.id, 'go-run-id': go.id, revision: source.revision,
    'head-sha': source.root.head_sha, 'pr-number': source.pr?.number ?? '', branch: source.root.head_branch };
}

// Runs the Go distribution's coverage tool on data only. No PR scripts, binaries,
// Go packages, or configuration files execute in this trusted upload workflow.
export function mergeCoverage(directory) {
  const inputs = [];
  for (let i = 1; i <= 10; i++) {
    const path = join(directory, `acceptance-coverage-linux-shard-${i}`);
    if (!readdirSync(path).some(name => name.startsWith('covmeta.'))) throw new Error(`Missing coverage shard ${i}`);
    inputs.push(path);
  }
  const merged = join(directory, 'merged');
  mkdirSync(merged, { recursive: true });
  const options = { cwd: directory, stdio: 'inherit', env: { ...process.env, GOWORK: 'off', GOFLAGS: '', GOTOOLCHAIN: 'local' } };
  execFileSync('go', ['tool', 'covdata', 'merge', '-pcombine', `-i=${inputs.join(',')}`, `-o=${merged}`], options);
  const raw = join(directory, 'raw.out');
  execFileSync('go', ['tool', 'covdata', 'textfmt', `-i=${merged}`, `-o=${raw}`], options);
  writeFileSync(join(directory, 'coverage.out'), readFileSync(raw, 'utf8').split('\n').filter(line => !line.includes('mock_')).join('\n'));
}
