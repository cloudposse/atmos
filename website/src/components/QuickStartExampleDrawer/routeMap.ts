export interface QuickStartExampleRoute {
  route: string;
  exampleName: 'quick-start-simple' | 'quick-start-advanced';
  selectedPath: string;
  relatedPaths?: string[];
}

export interface QuickStartExampleConfig {
  exampleName: QuickStartExampleRoute['exampleName'];
  selectedPath: string;
  relatedPaths: string[];
}

function examplePath(exampleName: QuickStartExampleRoute['exampleName'], path: string): string {
  return `${exampleName}/${path}`;
}

function route(
  path: string,
  exampleName: QuickStartExampleRoute['exampleName'],
  selectedFile: string,
  relatedFiles: string[] = []
): QuickStartExampleRoute {
  return {
    route: path,
    exampleName,
    selectedPath: examplePath(exampleName, selectedFile),
    relatedPaths: relatedFiles.map((file) => examplePath(exampleName, file)),
  };
}

const simple = 'quick-start-simple';
const advanced = 'quick-start-advanced';

export const QUICK_START_EXAMPLE_ROUTES: QuickStartExampleRoute[] = [
  route('/quick-start/simple/configure-cli', simple, 'atmos.yaml'),
  route('/quick-start/simple/configure-project', simple, 'README.md', ['atmos.yaml']),
  route('/quick-start/simple/write-components', simple, 'components/terraform/weather/main.tf', [
    'components/terraform/weather/variables.tf',
    'components/terraform/weather/main.tf',
    'components/terraform/weather/versions.tf',
    'components/terraform/weather/outputs.tf',
  ]),
  route('/quick-start/simple/configure-stacks', simple, 'stacks/catalog/station.yaml', [
    'stacks/catalog/station.yaml',
    'stacks/deploy/dev.yaml',
    'stacks/deploy/staging.yaml',
    'stacks/deploy/prod.yaml',
  ]),
  route('/quick-start/simple/provision', simple, 'README.md', [
    'README.md',
    'stacks/deploy/dev.yaml',
  ]),
  route('/quick-start/simple/summary', simple, 'README.md'),
  route('/quick-start/simple/extra-credit/add-another-component', simple, 'stacks/deploy/dev.yaml', [
    'stacks/deploy/dev.yaml',
    'stacks/catalog/station.yaml',
  ]),
  route('/quick-start/simple/extra-credit/add-custom-commands', simple, 'atmos.yaml'),
  route('/quick-start/simple/extra-credit/create-workflows', simple, 'atmos.yaml'),
  route('/quick-start/simple/extra-credit/vendor-components', simple, 'README.md'),
  route('/quick-start/simple/extra-credit', simple, 'README.md'),
  route('/quick-start/simple', simple, 'README.md'),

  route('/quick-start/advanced/configure-project', advanced, 'atmos.yaml'),
  route('/quick-start/advanced/install-toolchain', advanced, '.tool-versions', [
    '.tool-versions',
    '.atmos.d/test.yaml',
    'atmos.yaml',
  ]),
  route('/quick-start/advanced/start-sandbox', advanced, 'stacks/catalog/emulator/aws.yaml', [
    'stacks/catalog/emulator/aws.yaml',
    'atmos.yaml',
  ]),
  route('/quick-start/advanced/create-atmos-stacks', advanced, 'stacks/catalog/backend.yaml', [
    'stacks/catalog/kms-key/defaults.yaml',
    'stacks/catalog/s3-bucket/defaults.yaml',
    'stacks/catalog/dynamodb-table/defaults.yaml',
    'stacks/catalog/sns-topic/defaults.yaml',
    'stacks/catalog/sqs-queue/defaults.yaml',
    'stacks/catalog/app-config/defaults.yaml',
    'stacks/catalog/backend.yaml',
    'stacks/mixins/tenant/plat.yaml',
    'stacks/mixins/region/us-east-2.yaml',
    'stacks/mixins/stage/dev.yaml',
    'stacks/orgs/acme/plat/dev/us-east-2.yaml',
  ]),
  route('/quick-start/advanced/configure-hooks', advanced, 'stacks/orgs/acme/_defaults.yaml', [
    'stacks/orgs/acme/_defaults.yaml',
    'stacks/catalog/s3-bucket/defaults.yaml',
    'atmos.yaml',
  ]),
  route('/quick-start/advanced/configure-secrets', advanced, 'stacks/catalog/app-config/defaults.yaml', [
    'stacks/catalog/app-config/defaults.yaml',
    'atmos.yaml',
  ]),
  route('/quick-start/advanced/configure-validation', advanced, 'stacks/schemas/jsonschema/s3-bucket/validate-s3-bucket-component.json', [
    'atmos.yaml',
    'stacks/catalog/s3-bucket/defaults.yaml',
    'stacks/schemas/jsonschema/s3-bucket/validate-s3-bucket-component.json',
    'stacks/schemas/opa/s3-bucket/validate-s3-bucket-component.rego',
  ]),
  route('/quick-start/advanced/provision', advanced, 'README.md', [
    'README.md',
    'atmos.yaml',
    'stacks/catalog/backend.yaml',
  ]),
  route('/quick-start/advanced/create-workflows', advanced, 'stacks/workflows/backend.yaml'),
  route('/quick-start/advanced/add-custom-commands', advanced, 'atmos.yaml'),
  route('/quick-start/advanced/vendor-components', advanced, 'vendor.yaml'),
  route('/quick-start/advanced/configure-terraform-backend', advanced, 'atmos.yaml', [
    'atmos.yaml',
    'stacks/catalog/s3-bucket/defaults.yaml',
    'stacks/orgs/acme/_defaults.yaml',
    'stacks/orgs/acme/plat/dev/_defaults.yaml',
  ]),
  route('/quick-start/advanced/final-notes', advanced, 'README.md'),
  route('/quick-start/advanced/next-steps', advanced, 'README.md'),
  route('/quick-start/advanced', advanced, 'README.md'),
];

function normalizePathname(pathname: string): string {
  const clean = pathname.split(/[?#]/)[0] || '/';
  return clean.length > 1 ? clean.replace(/\/+$/, '') : clean;
}

function routeMatches(pathname: string, routePath: string): boolean {
  return pathname === routePath || pathname.startsWith(`${routePath}/`);
}

export function getQuickStartExampleConfig(pathname: string): QuickStartExampleConfig | null {
  const normalizedPathname = normalizePathname(pathname);
  const match = QUICK_START_EXAMPLE_ROUTES
    .slice()
    .sort((a, b) => b.route.length - a.route.length)
    .find((entry) => routeMatches(normalizedPathname, entry.route));

  if (!match) {
    return null;
  }

  return {
    exampleName: match.exampleName,
    selectedPath: match.selectedPath,
    relatedPaths: match.relatedPaths?.length ? match.relatedPaths : [match.selectedPath],
  };
}
