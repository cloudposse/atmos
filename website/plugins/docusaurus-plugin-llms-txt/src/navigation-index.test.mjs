import assert from 'node:assert/strict';
import {mkdtemp, readFile, writeFile, mkdir, rm} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import navigation from './navigation-index.js';

async function output(t) {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'atmos-navigation-'));
  t.after(() => rm(dir, {recursive: true, force: true}));
  return dir;
}
const doc = (routePath, title = routePath) => ({routePath, title});
const version = (sidebars, docs) => ({instanceId: 'default', versionName: 'current', sidebars, docs});
const metadata = (id, permalink) => ({id, permalink});

async function checkLinks(dir, content) {
  for (const [, url] of content.matchAll(/\]\(<(\/[^>]+\.md)>\)/g)) {
    if (url === '/index.md') continue;
    await assert.doesNotReject(readFile(path.join(dir, url.slice(1))), `Missing Markdown target: ${url}`);
  }
}

test('resolved sidebar categories keep labels, order and hierarchy across unrelated URL prefixes', async (t) => {
  const dir = await output(t);
  const documents = [doc('/intro/', 'Introduction'), doc('/migration/terraform', 'Terraform'), doc('/migration/make', 'Make')];
  const sidebars = {docs: [{type: 'category', label: 'Get Started', link: {type: 'doc', id: 'intro'}, items: [
    {type: 'category', label: 'Migration Guides', items: [
      {type: 'doc', id: 'terraform'},
      {type: 'category', label: 'Task Runners', items: [{type: 'doc', id: 'make'}]},
    ]},
  ]}]};
  const versions = [version(sidebars, [metadata('intro', '/intro/'), metadata('terraform', '/migration/terraform'), metadata('make', '/migration/make')])];
  const lines = await navigation.generateNavigationIndexes(documents, versions, dir, {
    baseUrl: '/', themeConfig: {navbar: {items: [{type: 'docSidebar', sidebarId: 'docs', label: 'Learn'}]}},
  });
  assert.match(lines[0], /^- \[Learn\].*3 pages/);
  assert.equal(lines[1], '  - [Get Started](</intro.md>) — 3 pages.');
  assert.match(lines[2], /^    - \[Migration Guides\]/);
  assert.match(lines[3], /^      - \[Task Runners\]/);
  assert.equal(lines.length, 4, 'Overview contains categories, not every leaf page');
  const url = lines[2].match(/<([^>]+)>/)[1];
  const index = await readFile(path.join(dir, url), 'utf8');
  assert.match(index, /\[Terraform\]\(<\/migration\/terraform.md>\)/);
  assert.match(index, /\[Task Runners\]/);
  assert.ok(index.indexOf('[Terraform]') < index.indexOf('[Task Runners]'));
  assert.doesNotMatch(index, /\[Make\]/, 'Nested leaves belong in their own category index');
});

test('unlisted pages and changelog get readable linked indexes with existing landing pages preserved', async (t) => {
  const dir = await output(t);
  const documents = [doc('/changelog/update', 'An update'), doc('/howto/task', 'A task'), doc('/cli/', 'CLI')];
  await mkdir(path.join(dir, 'cli'), {recursive: true});
  await writeFile(path.join(dir, 'cli.md'), '# Original CLI page');
  for (const d of documents.filter(d => d.routePath !== '/cli/')) {
    await mkdir(path.dirname(path.join(dir, d.routePath)), {recursive: true});
    await writeFile(path.join(dir, d.routePath + '.md'), d.title);
  }
  const lines = await navigation.generateNavigationIndexes(documents, [], dir, {baseUrl: '/'});
  assert.match(lines.join('\n'), /  - \[Changelog\].*1 page/);
  assert.match(lines.join('\n'), /  - \[How-to Guides\].*1 page/);
  assert.equal(await readFile(path.join(dir, 'cli.md'), 'utf8'), '# Original CLI page');
  await checkLinks(dir, lines.join('\n'));
  const url = lines.find(line => line.includes('[Changelog]')).match(/<([^>]+)>/)[1];
  const index = await readFile(path.join(dir, url), 'utf8');
  assert.match(index, /\[An update\]\(<\/changelog\/update.md>\)/);
  await checkLinks(dir, index);
});

test('aliases do not expand twice, links stay navigable, counts deduplicate pages, and empty categories disappear', async (t) => {
  const dir = await output(t);
  const versions = [version({cli: [
    {type: 'html', value: '<h3>Visual separator</h3>'},
    {type: 'category', label: 'Empty', items: []},
    {type: 'category', label: 'Commands', items: [
      {type: 'doc', id: 'plan'}, {type: 'doc', id: 'plan'},
      {type: 'link', label: 'Website', href: 'https://example.com'},
    ]},
    {type: 'category', label: 'Alias', customProps: {navigationReference: true}, items: [{type: 'doc', id: 'plan'}]},
  ]}, [metadata('plan', '/terraform/plan')])];
  const lines = await navigation.generateNavigationIndexes([doc('/terraform/plan', 'Plan')], versions, dir, {});
  assert.match(lines[0], /1 page/);
  assert.equal(lines.length, 2);
  const url = lines[1].match(/<([^>]+)>/)[1];
  assert.match(await readFile(path.join(dir, url), 'utf8'), /\[Website\]\(<https:\/\/example.com>\)/);
});

test('base URLs, Markdown labels and generated-path collisions are safe', async (t) => {
  const dir = await output(t);
  const occupied = '/docs/_markdown/default/current/cli';
  const documents = [doc('/docs/cli/plan', 'Plan'), doc(occupied, 'Real page')];
  const versions = [version({cli: [{type: 'category', label: 'CLI [tools]', items: [{type: 'doc', id: 'plan'}]}]}, [metadata('plan', '/docs/cli/plan')])];
  await mkdir(path.dirname(path.join(dir, occupied)), {recursive: true});
  await writeFile(path.join(dir, occupied + '.md'), '# Real page');
  const lines = await navigation.generateNavigationIndexes(documents, versions, dir, {baseUrl: '/docs/'});
  assert.match(lines[0], /\/cli-index.md/);
  assert.ok(lines[1].includes('CLI \\[tools\\]'));
  assert.equal(await readFile(path.join(dir, occupied + '.md'), 'utf8'), '# Real page');
  const index = await readFile(path.join(dir, occupied + '-index.md'), 'utf8');
  assert.match(index, /\[Documentation index\]\(<\/docs\/index.md>\)/);
});

test('plugin lifecycle exports resolved navigation and honors authoritative document permalinks', async (t) => {
  const {default: pluginFactory} = await import('./index.js');
  const dir = await output(t);
  const outDir = path.join(dir, 'build');
  const cacheDir = path.join(dir, '.docusaurus/docusaurus-plugin-content-docs/default');
  await mkdir(cacheDir, {recursive: true});
  await mkdir(path.join(dir, 'docs'));
  await mkdir(path.join(dir, 'blog'));
  await mkdir(outDir);
  await writeFile(path.join(dir, 'docs/page.md'), '---\ntitle: Exported page\n---\n\nContent.\n');
  await writeFile(path.join(cacheDir, 'page.json'), JSON.stringify({permalink: '/renamed/page', source: '@site/docs/page.md'}));
  const plugin = pluginFactory({siteDir: dir}, {generateLlmsTxt: false, generateLlmsFullTxt: false, generatePerPageMarkdown: true});
  plugin.allContentLoaded({allContent: {'docusaurus-plugin-content-docs': {default: {loadedVersions: [{
    versionName: 'current', docs: [metadata('page', '/renamed/page')],
    sidebars: {docs: [{type: 'category', label: 'Guides', items: [{type: 'doc', id: 'page'}]}]},
  }]}}}});
  const props = {outDir, routesPaths: ['/renamed/page'], siteConfig: {title: 'Atmos', url: 'https://atmos.tools', baseUrl: '/'}};
  await plugin.postBuild(props);
  const index = await readFile(path.join(outDir, 'index.md'), 'utf8');
  assert.match(index, /  - \[Guides\]/);
  await checkLinks(outDir, index);
  const guides = await readFile(path.join(outDir, '_markdown/default/current/docs/0-guides.md'), 'utf8');
  assert.match(guides, /\[Exported page\]\(<\/renamed\/page.md>\)/);
  await checkLinks(outDir, guides);

  // A site with an actual root document must retain that document's content.
  await writeFile(path.join(cacheDir, 'root.json'), JSON.stringify({permalink: '/', source: '@site/docs/page.md'}));
  await plugin.postBuild({...props, routesPaths: ['/', '/renamed/page']});
  assert.equal(await readFile(path.join(outDir, 'index.md'), 'utf8'), '# Exported page\n\nContent.\n');
});
