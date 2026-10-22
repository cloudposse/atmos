// Shared by the MDX component and plain-Markdown documentation export.
// Keep these consumers aligned with pkg/function/tag/scope.go.
const locations = {
  config: '**CLI configuration** (`atmos.yaml`, `atmos.d`, and profiles): during configuration loading.',
  stack: '**Resolved stack values**: after inheritance, merging, and component template processing.',
  scaffold: '**Scaffold manifests**: during manifest loading.',
  workflowPrompts: '**Workflow prompt fields**: `default`, `prompt`, `placeholder`, and `options` where supported by the `input`, `confirm`, `choose`, `filter`, `file`, and `write` step types, before the step executes.',
  configLoading: '**CLI configuration** (`atmos.yaml`, `atmos.d`, and profiles): during file loading.',
  stackLoading: '**Stack manifests**: during file loading, before inheritance and merging.',
  workflowLoading: '**Workflow manifests**: during manifest loading.',
  configMerge: '**CLI configuration** (`atmos.yaml`, `atmos.d`, and profiles): on sequences during configuration merging.',
  stackMerge: '**Stack manifests**: on sequences during inheritance and merging.',
  skillRef: '**Skill source refs** (`ai.skills.<name>.ref` in CLI configuration): resolved when skill sources are installed or synchronized.',
};

const scopes = {
  configAndStack: { locations: ['config', 'stack', 'scaffold'] },
  env: { locations: ['config', 'stack', 'workflowPrompts', 'scaffold'] },
  exec: { locations: ['config', 'stack', 'workflowPrompts'] },
  stack: { locations: ['stack'] },
  unset: {
    locations: ['config', 'stack'],
    note: 'Marks inherited values for removal.',
  },
  include: {
    locations: ['configLoading', 'stackLoading', 'workflowLoading', 'scaffold'],
    note: 'The path cannot depend on the final merged component context.',
  },
  literal: {
    locations: ['configLoading', 'stackLoading', 'workflowLoading', 'scaffold'],
    note: 'Stack scalar values retain protection through templates and YAML-function processing. Step execution honors only the literal-aware fields documented below.',
  },
  append: {
    locations: ['configMerge', 'stackMerge'],
    note: 'This is a merge directive, rather than a post-template value function.',
  },
  version: { locations: ['stack', 'skillRef'] },
  random: {
    locations: ['config', 'stack', 'scaffold'],
    note: 'Each evaluation generates a value; it is not saved as stable infrastructure state.',
  },
};

const functions = {
  append: 'append',
  'aws.account-id': 'stack',
  'aws.caller-identity-arn': 'stack',
  'aws.caller-identity-user-id': 'stack',
  'aws.cloudformation.output': 'stack',
  'aws.organization-id': 'stack',
  'aws.region': 'stack',
  'ci.labels': 'stack',
  cwd: 'configAndStack',
  emulator: 'stack',
  env: 'env',
  exec: 'exec',
  'git.branch': 'configAndStack',
  'git.host': 'configAndStack',
  'git.name': 'configAndStack',
  'git.owner': 'configAndStack',
  'git.ref': 'configAndStack',
  'git.repository': 'configAndStack',
  'git.root': 'configAndStack',
  'git.sha': 'configAndStack',
  'git.url': 'configAndStack',
  include: 'include',
  'include.raw': 'include',
  labels: 'stack',
  'labels.keys': 'stack',
  'labels.values': 'stack',
  literal: 'literal',
  random: 'random',
  'repo-root': 'configAndStack',
  secret: 'stack',
  starlark: 'stack',
  store: 'stack',
  'store.get': 'stack',
  tags: 'stack',
  template: 'stack',
  'terraform.output': 'stack',
  'terraform.state': 'stack',
  unset: 'unset',
  version: 'version',
};

export function availabilityMarkdown(name) {
  if (!Object.hasOwn(functions, name)) {
    throw new Error(`Unknown YAML function availability: ${name}`);
  }
  const scope = scopes[functions[name]];
  return [
    '## Availability and evaluation',
    'Supported locations:',
    scope.locations.map((location) => `- ${locations[location]}`).join('\n'),
    scope.note,
    'See [YAML function scope](/functions/yaml/scope) for field restrictions, import timing, and native tags versus quoted strings.',
  ].filter(Boolean).join('\n\n') + '\n';
}
