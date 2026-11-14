import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

// Fixed local emulator endpoints, not supplied by the template user.
const api = 'http://local-backstage-git:3000/api/v1/repos/atmos/deployments';
const browserRepo = 'http://localhost:3001/atmos/deployments';
const authorization = `Basic ${Buffer.from('atmos:atmos').toString('base64')}`;

async function request<T>(path: string, body?: unknown, method = 'POST'): Promise<T> {
  const response = await fetch(`${api}${path}`, {
    method: body === undefined ? 'GET' : method,
    headers: { Authorization: authorization, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(15_000),
  });
  if (!response.ok) {
    throw new Error(`Local Gitea ${path}: HTTP ${response.status}: ${await response.text()}`);
  }
  return response.json() as Promise<T>;
}

async function exists(path: string): Promise<boolean> {
  const response = await fetch(`${api}${path}`, {
    headers: { Authorization: authorization },
    signal: AbortSignal.timeout(15_000),
  });
  if (response.status === 404) return false;
  if (!response.ok) throw new Error(`Local Gitea ${path}: HTTP ${response.status}`);
  return true;
}

export function createServiceRequestAction() {
  return createTemplateAction({
    id: 'atmos:request-service',
    description: 'Open a local infrastructure PR; provisioning happens after review and merge.',
    schema: {
      input: {
        name: z => z.string().min(3).max(30).regex(/^[a-z][a-z0-9-]*[a-z0-9]$/),
      },
      output: { pullRequestUrl: z => z.string() },
    },
    async handler(ctx) {
      const name = ctx.input.name;
      // Reserved by catalog/entities.yaml as component:default/payments.
      if (name === 'payments') {
        throw new Error('Service payments already exists in the seed catalog. Choose a new name.');
      }
      const branch = `request-${name}`;
      const stackPath = `stacks/deploy/services/${name}.yaml`;
      if (await exists(`/contents/${stackPath}?ref=main`)) {
        throw new Error(`Service ${name} already exists on main. Choose a new name.`);
      }
      if (!(await exists(`/branches/${branch}`))) {
        await request('/branches', { new_branch_name: branch, old_branch_name: 'main' });
      }
      // Each task gets an isolated Atmos checkout, including concurrent requests.
      // Atmos owns rendering and Git publication; this adapter supplies task context.
      await writeFile(join(ctx.workspacePath, 'atmos.yaml'), JSON.stringify({
        base_path: '.',
        git: { repositories: { request: {
          uri: 'http://atmos:atmos@local-backstage-git:3000/atmos/deployments.git',
          branch, workdir: 'repository',
          commit: { signing: 'never', author: { name: 'Atmos Portal', email: 'portal@localhost' } },
        } } },
        commands: [{ name: 'request', flags: [{ name: 'service', required: true }],
          steps: [{ type: 'script', source: '/demo/scripts/request.star' }],
        }],
      }));
      const result = await promisify(execFile)('/usr/local/bin/atmos',
        ['request', '--service', name],
        { cwd: ctx.workspacePath, timeout: 120_000, maxBuffer: 2 * 1024 * 1024 });
      ctx.logger.info(result.stdout);
      ctx.logger.info(result.stderr);
      const pulls = await request<Array<{ number: number; head: { ref: string } }>>('/pulls?state=open&limit=50');
      const existing = pulls.find(pull => pull.head.ref === branch);
      const pull = existing ?? await request<{ number: number }>('/pulls', {
        base: 'main', head: branch, title: `Provision ${name} with Atmos`,
        body: `Review the Atmos stack and service catalog entry. Wait for the plan check before merging. After merge, Gitea Actions provisions the local bucket and registers the service. This demo targets only the local AWS emulator.`,
      });
      const url = `${browserRepo}/pulls/${pull.number}`;
      ctx.logger.info(`Review required: ${url}`);
      ctx.output('pullRequestUrl', url);
    },
  });
}
