const matter = require('gray-matter');
const {inputFields} = require('./policy-input-fields');

const engines = ['cel', 'starlark'];

function policyVariants(example, primary) {
  const directory = example.root.children.find(node => node.name === 'variants');
  const files = [{file: primary, path: 'policy.yaml'}];
  for (const child of directory?.children ?? []) {
    const file = child.children?.find(node => node.name === 'policy.yaml');
    if (!engines.includes(child.name) || !file?.content) {
      throw new Error(`Invalid policy variant: ${example.name}/${child.name}`);
    }
    files.push({file, path: `variants/${child.name}/policy.yaml`, expectedEngine: child.name});
  }
  const variants = files.map(({file, path, expectedEngine}) => {
    const document = matter.engines.yaml.parse(file.content);
    const rules = Object.values(document?.spec?.policies ?? {});
    const engine = rules[0]?.engine ?? 'cel';
    if (rules.length !== 1 || !engines.includes(engine) || (expectedEngine && engine !== expectedEngine)) {
      throw new Error(`Invalid policy engine: ${example.name}/${path}`);
    }
    return {engine, path, policy: file.content, githubUrl: file.githubUrl};
  });
  if (variants.length !== engines.length || new Set(variants.map(variant => variant.engine)).size !== engines.length) {
    throw new Error(`Policy ${example.name} needs exactly one CEL and one Starlark implementation`);
  }
  return variants.sort((a, b) => engines.indexOf(a.engine) - engines.indexOf(b.engine));
}

/** Build code-first pages from the same manifests and fixtures tested by Go. */
function policyPage(example) {
  const file = example.root.children.find(node => node.name === 'policy.yaml');
  const tests = example.root.children.find(node => node.name === 'tests');
  const cases = tests?.children?.find(node => node.name === 'cases.json');
  if (!file?.content || !cases?.content || !example.root.readme?.content) {
    throw new Error(`Incomplete policy library entry: ${example.name}`);
  }
  const {data, content} = matter(example.root.readme.content);
  const sections = Object.fromEntries([...content.matchAll(/^## ([^\n]+)\n([\s\S]*?)(?=^## |$(?![\s\S]))/gm)]
    .map(([, heading, body]) => [heading, body.trim()]));
  if (!sections.Configuration || !sections.Contract || !data.command) {
    throw new Error(`Policy ${example.name} needs Contract, Configuration, and command metadata`);
  }
  const fixtures = JSON.parse(cases.content);
  return {
    name: example.name,
    title: example.title,
    description: example.description,
    tags: example.tags,
    docs: example.docs,
    variants: policyVariants(example, file),
    configuration: sections.Configuration,
    contract: sections.Contract,
    inputReference: sections.Inputs ?? '',
    command: data.command,
    cases: fixtures,
    caseFields: fixtures.map(fixture => inputFields(fixture.request)),
  };
}

module.exports = {policyPage};
