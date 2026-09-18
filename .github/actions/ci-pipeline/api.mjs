// Small dependency-free REST client. Never execute or import code from a test checkout.
export class GitHub {
  constructor(repo, token, transport = fetch, sleep = ms => new Promise(resolve => setTimeout(resolve, ms))) {
    if (!/^[\w.-]+\/[\w.-]+$/.test(repo)) throw new Error('Invalid repository');
    this.repo = repo;
    this.token = token;
    this.transport = transport;
    this.sleep = sleep;
  }

  async request(method, path, body) {
    for (let attempt = 0; ; attempt++) {
      const response = await this.transport(`https://api.github.com/repos/${this.repo}${path}`, {
        method,
        headers: { Authorization: `Bearer ${this.token}`, Accept: 'application/vnd.github+json',
          'Content-Type': 'application/json', 'X-GitHub-Api-Version': '2022-11-28' },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: AbortSignal.timeout(30000),
      });
      if (response.ok) return response.status === 204 ? {} : response.json();
      if (method === 'GET' && attempt < 2 && (response.status >= 500 || response.status === 429)) {
        await this.sleep(1000 * 2 ** attempt);
        continue;
      }
      throw Object.assign(new Error(`GitHub ${method} ${path}: ${response.status}`), { status: response.status });
    }
  }

  async list(path, field) {
    const result = [];
    for (let page = 1; page <= 100; page++) {
      const data = await this.request('GET', `${path}${path.includes('?') ? '&' : '?'}per_page=100&page=${page}`);
      const items = field ? data[field] : data;
      if (!Array.isArray(items)) throw new Error(`Invalid collection: ${path}`);
      result.push(...items);
      if (items.length < 100) return result;
    }
    throw new Error(`Pagination limit exceeded: ${path}`);
  }

  run(id) { return this.request('GET', `/actions/runs/${integer(id)}`); }
  async jobs(run, expected = []) {
    for (let attempt = 0; ; attempt++) {
      const jobs = await this.list(`/actions/runs/${integer(run.id)}/jobs?filter=latest`, 'jobs');
      const incomplete = jobs.length === 0 || jobs.some(job => job.status !== 'completed' || !job.conclusion) ||
        expected.some(name => !jobs.some(job => job.name === name));
      if (run.status !== 'completed' || !incomplete || attempt === 2) return jobs;
      // Completion webhooks can precede the jobs API's final updates.
      await this.sleep(10000);
    }
  }
}

export function integer(value) {
  const n = Number(value);
  if (!Number.isSafeInteger(n) || n < 1) throw new Error(`Invalid positive integer: ${value}`);
  return n;
}
