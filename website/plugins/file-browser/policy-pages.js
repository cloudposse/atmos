const matter = require('gray-matter');

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
  if (!Array.isArray(fixtures) || fixtures.length === 0) {
    throw new Error(`Policy ${example.name} needs at least one test case`);
  }
  return {
    name: example.name,
    title: example.title,
    description: example.description,
    tags: example.tags,
    docs: example.docs,
    policy: file.content,
    githubUrl: file.githubUrl,
    configuration: sections.Configuration,
    contract: sections.Contract,
    command: data.command,
    cases: fixtures,
  };
}

module.exports = {policyPage};
