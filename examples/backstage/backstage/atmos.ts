import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { createBackendModule } from '@backstage/backend-plugin-api';
import {
  createTemplateAction,
  scaffolderActionsExtensionPoint,
} from '@backstage/plugin-scaffolder-node';

import { createServiceRequestAction } from './request-service';

const execute = promisify(execFile);

export default createBackendModule({
  pluginId: 'scaffolder',
  moduleId: 'atmos',
  register(env) {
    env.registerInit({
      deps: { scaffolder: scaffolderActionsExtensionPoint },
      async init({ scaffolder }) {
        scaffolder.addActions(
          createServiceRequestAction(),
          createTemplateAction({
            id: 'atmos:describe',
            description: 'Resolve the demo service configuration with Atmos.',
            schema: {
              input: {
                stack: z => z.enum(['dev', 'prod']),
              },
              output: {
                configuration: z => z.string(),
              },
            },
            async handler(ctx) {
              // No shell, arbitrary command, path, or user-supplied flags. The
              // server-side schema enforces the same allowlist as the UI.
              const args = [
                'describe', 'component', 'service',
                '--stack', ctx.input.stack, '--format', 'json',
              ];
              ctx.logger.info(`Running atmos ${args.join(' ')}`);
              const { stdout } = await execute('/usr/local/bin/atmos', args, {
                cwd: '/demo',
                timeout: 30_000,
                maxBuffer: 1024 * 1024,
                env: { ...process.env, ATMOS_LOGS_LEVEL: 'Off', NO_COLOR: '1' },
              });
              const configuration = JSON.stringify(JSON.parse(stdout), null, 2);
              ctx.logger.info(configuration);
              ctx.output('configuration', configuration);
            },
          }),
        );
      },
    });
  },
});
