import { readFileSync } from 'node:fs';
import { integer } from './api.mjs';

export const policy = JSON.parse(readFileSync(new URL('./policy.json', import.meta.url), 'utf8'));
export const roots = ['ci-build-linux.yml', 'ci-build-macos.yml', 'ci-build-windows.yml', 'ci-test-go.yml', 'ci-test-kubernetes.yml', 'ci-lint.yml'];
export const filename = run => run.path?.split('@')[0].split('/').at(-1);
export const sourceTitle = (id, attempt) => `CI source ${integer(id)} attempt ${integer(attempt)}`;

export function parentIdentity(run) {
  if (!policy[filename(run)] || run.event !== 'workflow_run') throw new Error('Not a CI child workflow');
  const match = /^CI source ([1-9]\d*) attempt ([1-9]\d*)$/.exec(run.display_title);
  if (!match) throw new Error('Missing CI parent identity');
  return { id: integer(match[1]), attempt: integer(match[2]) };
}

export function sourceIdentity(root) {
  if (!roots.includes(filename(root)) || !['pull_request', 'push', 'merge_group', 'workflow_dispatch'].includes(root.event)) {
    throw new Error('Unrecognized CI root');
  }
  const match = /^CI revision ([a-f0-9]{40}) base ([a-f0-9]{40})$/.exec(root.display_title);
  if (!match) throw new Error('Missing immutable source revision');
  return { revision: match[1], base: match[2] };
}

// A PR merge revision must contain the exact head that GitHub associates with
// the root run. Never accept a SHA or PR number supplied by downloaded test data.
export async function resolveRoot(api, run) {
  let root = run;
  if (run.event === 'workflow_run') {
    const parent = parentIdentity(run);
    root = await api.run(parent.id);
    if (filename(root) !== policy[filename(run)].parent || root.run_attempt !== parent.attempt) {
      return { active: false, root };
    }
  }
  const { revision, base: recordedBase } = sourceIdentity(root);
  let base = recordedBase;
  if (root.repository?.full_name !== api.repo) throw new Error('Source repository mismatch');
  if (revision !== root.head_sha) {
    if (root.event !== 'pull_request') throw new Error('Source SHA mismatch');
    const commit = await api.request('GET', `/commits/${revision}`);
    if (!commit.parents?.some(parent => parent.sha === root.head_sha)) throw new Error('PR merge revision has the wrong head');
  }
  if (base === '0'.repeat(40) || (root.event === 'workflow_dispatch' && base === revision)) {
    const defaultBranch = root.repository.default_branch || 'main';
    const comparison = await api.request('GET', `/commits/${encodeURIComponent(defaultBranch)}`);
    base = comparison.sha;
    if (!/^[a-f0-9]{40}$/.test(base)) throw new Error('Invalid comparison base');
  }
  let pr;
  if (root.event === 'pull_request') {
    const associated = root.pull_requests?.length ? root.pull_requests : await api.list(`/commits/${root.head_sha}/pulls`);
    for (const candidate of associated) {
      const current = await api.request('GET', `/pulls/${integer(candidate.number)}`);
      if (current.state === 'open' && current.head.sha === root.head_sha && current.base.repo.full_name === api.repo) {
        pr = current;
        break;
      }
    }
    if (!pr) return { active: false, root };
  }
  const siblings = await api.list(`/actions/workflows/${filename(root)}/runs?head_sha=${root.head_sha}&event=${root.event}`, 'workflow_runs');
  // A newer run on the same head (e.g. ready_for_review or a new base merge)
  // supersedes older artifacts even if the PR head itself has not moved.
  if (siblings.some(candidate => candidate.id > root.id)) return { active: false, root };
  return { active: true, root, revision, base, pr, event: root.event, draft: Boolean(pr?.draft) };
}

export async function childRun(api, file, root) {
  const title = sourceTitle(root.id, root.run_attempt);
  const runs = await api.list(`/actions/workflows/${file}/runs?event=workflow_run&created=${encodeURIComponent('>=' + root.created_at)}`, 'workflow_runs');
  return runs.filter(run => run.display_title === title && filename(run) === file)
    .sort((a, b) => b.id - a.id)[0];
}

export async function peerRoot(api, file, source) {
  const runs = await api.list(`/actions/workflows/${file}/runs?head_sha=${source.root.head_sha}&event=${source.root.event}`, 'workflow_runs');
  const run = runs.sort((a, b) => b.id - a.id)[0];
  if (!run) return undefined;
  const peer = await resolveRoot(api, run);
  return peer.active && peer.revision === source.revision ? peer.root : undefined;
}
