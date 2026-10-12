import assert from 'node:assert/strict';
import {readFileSync, readdirSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import test from 'node:test';
import matter from 'gray-matter';
import plugin from './index.js';

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
  });
  const content = await browser.loadContent();
  const entries = readdirSync(libraryDir, {withFileTypes: true}).filter(entry => entry.isDirectory());
  assert.ok(entries.length > 0, 'The library must not be empty');
  assert.deepEqual(content.tree.examples.map(entry => entry.name).sort(), entries.map(entry => entry.name).sort());
  assert.deepEqual(content.tree.tags, ['Terraform', 'GitHub Actions', 'Lifecycle Hooks']);
  assert.equal(content.options.searchable, true);
  assert.equal(content.options.installCommandTemplate, '', 'Do not invent a policy install command');
  assert.equal(content.options.enableCopyMarkdown, false, 'Keep the first-pass gallery controls minimal');

  const routes = [];
  await browser.contentLoaded({content, actions: {
    createData: async name => `data/${name}`,
    addRoute: route => routes.push(route),
    setGlobalData: () => {},
  }});
  const paths = new Set(routes.map(route => route.path));
  assert.equal(paths.size, routes.length, 'Generated routes must be unique');
  assert.ok(paths.has('/policies/library'));
  const configuration = readFileSync(path.join(siteDir, 'docs/policies/configuration.mdx'), 'utf8');
  assert.match(configuration, /^## Enforcement$/m);

  for (const entry of content.tree.examples) {
    const base = `/policies/library/${entry.name}`;
    for (const suffix of ['', '/policy.yaml', '/tests/cases.json']) {
      assert.ok(paths.has(base + suffix), `Missing catalog route: ${base + suffix}`);
    }
    const {data, content: readme} = matter(entry.root.readme.content);
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
