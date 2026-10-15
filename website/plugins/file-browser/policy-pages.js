const matter = require('gray-matter');
const {inputFields} = require('./policy-input-fields');

const engines = ['cel', 'starlark'];

/** Extract catalog metadata from a single-rule manifest without including its source or fixtures. */
function policySummary(example) {
  const file = example.root.children.find(node => node.name === 'policy.yaml');
  const document = matter.engines.yaml.parse(file?.content ?? '');
  const rules = Object.values(document?.spec?.policies ?? {});
  if (rules.length !== 1 || !rules[0].on || !rules[0].action) {
    throw new Error(`Policy ${example.name} needs one rule with an input and action`);
  }
  const {name, title, description, tags} = example;
  return {name, title, description, tags, inputKind: rules[0].on, action: rules[0].action};
}

/** Require one implementation per supported engine and return them in selector order. */
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
    return {engine, path, policy: file.content, githubUrl: file.githubUrl, reason: rules[0].reason?.trim() || undefined};
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
    ...policySummary(example),
    docs: example.docs,
    variants: policyVariants(example, file),
    configuration: sections.Configuration,
    contract: sections.Contract,
    inputReference: sections.Inputs ?? '',
    command: data.command,
    caseLabels: data.scenarios ?? {},
    cases: fixtures,
    caseFields: fixtures.map(fixture => inputFields(fixture.request)),
  };
}

/** Focus the owning docs section on library entries while retaining the rest of Reference. */
function policySidebar(items, examples, routeBasePath) {
  return items.map(item => {
    if (item.type !== 'category' || !item.items.some(child => child.href === routeBasePath)) return item;
    return {
      ...item,
      label: 'Policy Library',
      href: routeBasePath,
      items: [
        {type: 'link', label: '← Policy guides', href: item.href},
        ...examples.map(example => ({
          type: 'link', label: example.title, href: `${routeBasePath}/${example.name}`,
          customProps: {policyLibraryEntry: true, navigationMatchPrefix: true},
        })),
      ],
    };
  });
}

module.exports = {policyPage, policySummary, policySidebar};
