import assert from 'node:assert/strict';
import {readFileSync, readdirSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import test from 'node:test';
import matter from 'gray-matter';
import plugin from './index.js';
import {policyPage} from './policy-pages.js';

const siteDir = fileURLToPath(new URL('../../', import.meta.url));
const libraryDir = path.resolve(siteDir, '../policy-library');

test('the policy gallery discovers individually documented and tested policies', async () => {
  const browser = plugin({siteDir}, {
    id: 'policies',
    sourceDir: '../policy-library',
    routeBasePath: '/policies/library',
    title: 'Policy Library',
    tagOrder: ['Terraform', 'GitHub Actions', 'Lifecycle Hooks'],
    searchable: true,
    cardCtaLabel: 'View policy',
    enablePerPageMarkdown: true,
    presentation: 'policies',
    docsSidebar: 'cli',
  });
  const content = await browser.loadContent();
  const entries = readdirSync(libraryDir, {withFileTypes: true}).filter(entry => entry.isDirectory());
  assert.ok(entries.length > 0, 'The library must not be empty');
  assert.deepEqual(content.tree.examples.map(entry => entry.name).sort(), entries.map(entry => entry.name).sort());
  assert.deepEqual(content.tree.tags, ['Terraform', 'GitHub Actions', 'Lifecycle Hooks', 'Affected Changes']);
  assert.equal(content.options.searchable, true);
  assert.equal(content.options.installCommandTemplate, '', 'Do not invent a policy install command');
  assert.equal(content.options.enableCopyMarkdown, false, 'Keep the first-pass gallery controls minimal');

  const routes = [];
  const modules = new Map();
  await browser.contentLoaded({content, actions: {
    createData: async (name, content) => { modules.set(`data/${name}`, JSON.parse(content)); return `data/${name}`; },
    addRoute: route => routes.push(route),
    setGlobalData: () => {},
  }});
  const paths = new Set(routes.map(route => route.path));
  assert.equal(paths.size, routes.length, 'Generated routes must be unique');
  assert.ok(paths.has('/policies/library'));
  assert.match(routes[0].component, /PolicyLibrary\/IndexPage/);
  assert.equal(modules.get(routes[0].modules.treeData).examples[0].root, undefined, 'Index must not bundle every manifest and test');
  const configuration = readFileSync(path.join(siteDir, 'docs/policies/configuration.mdx'), 'utf8');
  assert.match(configuration, /^## Enforcement$/m);

  for (const entry of content.tree.examples) {
    const base = `/policies/library/${entry.name}`;
    const route = routes.find(route => route.path === base);
    assert.match(route.component, /PolicyLibrary\/PolicyPage/);
    const page = modules.get(route.modules.policyData);
    assert.equal(page.policy, readFileSync(path.join(libraryDir, entry.name, 'policy.yaml'), 'utf8'));
    assert.deepEqual(page.cases, JSON.parse(readFileSync(path.join(libraryDir, entry.name, 'tests/cases.json'), 'utf8')));
    assert.ok(page.configuration && page.contract);
    assert.match(page.command, /^atmos \S/);
    for (const suffix of ['', '/policy.yaml', '/tests/cases.json']) {
      assert.ok(paths.has(base + suffix), `Missing catalog route: ${base + suffix}`);
    }
    const {data, content: readme} = matter(entry.root.readme.content);
    assert.equal(page.command, data.command, 'Use the documented command, including repository-wide commands');
    assert.ok(data.title && data.description && data.tags.length, `${entry.name}: catalog metadata required`);
    assert.ok(entry.docs.length >= 2, `${entry.name}: reference links required`);
    for (const section of ['Contract', 'Configuration', 'Policy', 'Use', 'Tests']) {
      assert.ok(readme.includes(`## ${section}\n`), `${entry.name}: missing ${section}`);
    }
    const links = [...readme.matchAll(/\]\(([^)]+)\)/g)].map(match => match[1]);
    for (const target of [`${base}/policy.yaml`, `${base}/tests/cases.json`, '/policies/configuration#enforcement']) {
      assert.ok(links.includes(`https://atmos.tools${target}`), `${entry.name}: missing documentation link ${target}`);
    }
    for (const link of links) {
      // READMEs also render on GitHub and are checked outside Docusaurus by Lychee.
      // Root-relative website routes are interpreted as missing repository files there.
      assert.ok(!link.startsWith('/'), `${entry.name}: use an absolute documentation URL: ${link}`);
      if (link.startsWith('https://atmos.tools/policies/library/')) {
        const url = new URL(link);
        assert.ok(paths.has(url.pathname), `${entry.name}: broken library link ${link}`);
      }
    }
    const fixtures = JSON.parse(readFileSync(path.join(libraryDir, entry.name, 'tests/cases.json'), 'utf8'));
    assert.ok(fixtures.some(fixture => ['deny', 'allow'].includes(fixture.status)), `${entry.name}: matching case required`);
    assert.ok(fixtures.some(fixture => fixture.status === 'not_applicable'), `${entry.name}: nonmatching case required`);
  }
});

test('documentation navigation uses resolved docs links and fails on a missing sidebar', async () => {
  const browser = plugin({siteDir}, {id: 'policies', docsSidebar: 'cli'});
  const content = {tree: {examples: []}, options: {id: 'policies'}};
  let global;
  const actions = {setGlobalData: value => {global = value;}};
  const allContent = {'file-browser': {policies: content}, 'docusaurus-plugin-content-docs': {default: {loadedVersions: [{
    versionName: 'current',
    sidebars: {cli: [{type: 'doc', id: 'policies/index'}]},
    docs: [{id: 'policies/index', title: 'Policies', permalink: '/policies', frontMatter: {}}],
  }]}}};
  await browser.allContentLoaded({allContent, actions});
  assert.equal(global.sidebar.name, 'cli');
  assert.equal(global.sidebar.items[0].href, '/policies');
  await assert.rejects(browser.allContentLoaded({allContent: {'file-browser': {policies: content}}, actions}), /sidebar not found/);
  await plugin({siteDir}, {}).allContentLoaded({allContent: {}, actions: {setGlobalData: () => assert.fail('Standalone galleries must not acquire docs navigation')}});
});

test('incomplete policy pages fail the build rather than publishing missing code', () => {
  assert.throws(() => policyPage({name: 'missing', root: {children: []}}), /Incomplete policy/);
});

test('a missing source directory still yields the options the route lifecycle reads', async () => {
  const browser = plugin({siteDir}, {
    id: 'missing',
    sourceDir: '../no-such-directory',
    routeBasePath: '/missing',
    title: 'Missing',
    description: 'No source',
    docsSidebar: 'cli',
  });
  const content = await browser.loadContent();
  assert.deepEqual(content.tree.examples, []);
  assert.equal(content.options.id, 'missing');
  assert.equal(content.options.routeBasePath, '/missing');
  assert.equal(content.options.docsSidebar, 'cli');
});

test('a policy page needs at least one fixture so previews cannot render an empty example list', () => {
  const readme = '---\ncommand: atmos terraform plan vpc -s prod\n---\n\n## Contract\n\nA contract.\n\n## Configuration\n\nA configuration.\n';
  const example = cases => ({
    name: 'empty',
    root: {
      readme: {content: readme},
      children: [
        {name: 'policy.yaml', content: 'apiVersion: atmos/v1\n'},
        {name: 'tests', children: [{name: 'cases.json', content: cases}]},
      ],
    },
  });
  assert.throws(() => policyPage(example('[]')), /needs at least one test case/);
  assert.throws(() => policyPage(example('{}')), /needs at least one test case/);
  assert.equal(policyPage(example('[{"name": "denied"}]')).cases.length, 1);
});
