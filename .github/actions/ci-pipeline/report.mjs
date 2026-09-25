import { childRun, filename, peerRoot, policy, resolveRoot } from './source.mjs';

// Missing, duplicated, skipped or cancelled shards must never pass a required check.
// jobs?filter=latest includes successful jobs retained by a failed-jobs rerun.
export function resultForJobs(run, jobs, expected) {
  if (!run || run.status !== 'completed') return 'in_progress';
  return expected.every(name => {
    const matches = jobs.filter(job => job.name === name);
    return matches.length === 1 && matches[0].status === 'completed' && matches[0].conclusion === 'success';
  }) ? 'success' : 'failure';
}

export function combined(results) {
  if (results.includes('failure')) return 'failure';
  return results.every(value => value === 'success') ? 'success' : 'in_progress';
}

async function suite(api, file, root) {
  if (!root || root.status !== 'completed') return { result: 'in_progress', jobs: [] };
  if (root.conclusion !== 'success') return { result: 'failure', jobs: [] };
  const run = await childRun(api, file, root);
  const jobs = run ? await api.jobs(run, policy[file].jobs) : [];
  const result = run?.status === 'completed' && run.conclusion !== 'success' ? 'failure' : resultForJobs(run, jobs, policy[file].jobs);
  return { result, run, jobs };
}

export async function collectReports(api, source) {
  const reports = [];
  const rootFile = filename(source.root);
  for (const [file, definition] of Object.entries(policy)) {
    if (definition.parent !== rootFile) continue;
    const state = await suite(api, file, source.root);
    const url = state.run?.html_url ?? source.root.html_url;
    reports.push({ name: definition.name, result: state.result, url });
    for (const check of definition.required) {
      const result = state.result === 'failure' && !state.run ? 'failure' : resultForJobs(state.run, state.jobs, check.jobs);
      reports.push({ name: check.name, result, url });
    }
  }
  if (['ci-build-linux.yml', 'ci-build-macos.yml'].includes(rootFile)) {
    const results = [];
    for (const platform of ['linux', 'macos']) {
      const root = rootFile === `ci-build-${platform}.yml` ? source.root : await peerRoot(api, `ci-build-${platform}.yml`, source);
      const state = await suite(api, `ci-test-k3s-${platform}.yml`, root);
      // Historical gate included every fixture in both matrices, not just the demo.
      results.push(state.result);
    }
    reports.push({ name: '[k3s] demo-helmfile', result: combined(results), url: source.root.html_url });
  }
  return reports;
}

export async function publishReports(api, source, reports) {
  // API-derived revision is also the native build check SHA (PR merge commit or
  // merge queue synthetic commit). Never accidentally publish against main.
  const existing = await api.list(`/commits/${source.revision}/check-runs?filter=latest`, 'check_runs');
  for (const report of reports) {
    const external = `atmos-ci:${report.name}`;
    const previous = existing.find(check => check.name === report.name && check.external_id === external && check.app?.id === 15368);
    const finished = report.result !== 'in_progress';
    const body = {
      name: report.name, external_id: external, details_url: report.url,
      status: finished ? 'completed' : 'in_progress',
      output: { title: report.name, summary: `Source: ${source.revision}\n\n[Workflow results](${report.url})` },
      ...(finished ? { conclusion: report.result, completed_at: new Date().toISOString() } : {}),
    };
    if (previous?.status === body.status && (!finished || previous.conclusion === report.result) && previous.details_url === report.url) continue;
    if (previous) await api.request('PATCH', `/check-runs/${previous.id}`, body);
    else await api.request('POST', '/check-runs', { ...body, head_sha: source.revision });
  }
}

export async function report(api, triggering) {
  const source = await resolveRoot(api, triggering);
  if (!source.active) return { active: false };
  const reports = [];
  const observedRoots = [];
  for (const file of ['ci-build-linux.yml', 'ci-build-macos.yml', 'ci-build-windows.yml']) {
    const root = filename(source.root) === file ? source.root : await peerRoot(api, file, source);
    if (root) {
      observedRoots.push(root);
      reports.push(...await collectReports(api, { ...source, root }));
    }
  }
  const unique = [...new Map(reports.map(check => [check.name, check])).values()];
  // Recheck after querying siblings so a superseding push/rerun cannot publish
  // stale green results. The workflow serializes reporters for a source SHA.
  const latest = await resolveRoot(api, await api.run(triggering.id));
  if (!latest.active || latest.root.run_attempt !== source.root.run_attempt) return { active: false };
  for (const root of observedRoots) {
    const current = await resolveRoot(api, await api.run(root.id));
    if (!current.active || current.root.run_attempt !== root.run_attempt) return { active: false };
  }
  await publishReports(api, source, unique);
  return { active: true, revision: source.revision, reports };
}
