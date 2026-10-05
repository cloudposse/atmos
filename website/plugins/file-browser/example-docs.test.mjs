import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import matter from 'gray-matter';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import plugin from './index.js';

const siteDir = fileURLToPath(new URL('../../', import.meta.url));
const normalize = (url) => url.replace(/\/$/, '');

test('examples declare related documentation in front matter and link terms in prose', async () => {
  const { tree } = await plugin({ siteDir }, { sourceDir: '../examples' }).loadContent();
  assert.ok(tree.examples.length > 0);
  for (const example of tree.examples) {
    assert.ok(example.docs.length >= 2, `${example.name}: add relevant documentation links`);
    assert.ok(example.root.readme, `${example.name}: add a README explaining the example`);
    const { data, content } = matter(example.root.readme.content);
    assert.deepEqual(example.docs, data.related_docs, `${example.name}: declare related_docs in front matter`);
    const urls = example.docs.map(({ url }) => normalize(url));
    assert.equal(new Set(urls).size, urls.length, `${example.name}: duplicate documentation links`);
    const nodes = (node) => [node, ...(node.children || []).flatMap(nodes)];
    const body = nodes(unified().use(remarkParse).parse(content));
    assert.ok(body.some((node) => node.type === 'link' && node.url.startsWith('https://atmos.tools/')),
      `${example.name}: link relevant terms in the README prose`);
    assert.doesNotMatch(content, /^#{1,6}\s+Related Documentation\s*$/im,
      `${example.name}: related documentation is rendered from front matter, not a README chapter`);
    for (const { label, url } of example.docs) {
      assert.ok(label.trim(), `${example.name}: documentation link needs a label`);
      assert.ok(url.startsWith('/') && !url.startsWith('//'), `${example.name}: use a local documentation route`);
    }
  }
});

function fixture(t, readme) {
  const root = mkdtempSync(path.join(tmpdir(), 'atmos-related-docs-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'example'));
  writeFileSync(path.join(root, 'example/README.md'), readme);
  return plugin({ siteDir }, { sourceDir: root });
}

test('new examples supply their own documentation without a central mapping', async (t) => {
  const { tree } = await fixture(t, `---
related_docs:
  - label: Workflow configuration
    url: /workflows
---
# Example
`).loadContent();
  assert.deepEqual(tree.examples[0].docs, [{ label: 'Workflow configuration', url: '/workflows' }]);
});

test('items without related documentation retain an empty panel', async (t) => {
  const { tree } = await fixture(t, '# Example\n').loadContent();
  assert.deepEqual(tree.examples[0].docs, []);
});

test('malformed related documentation fails with the README path', async (t) => {
  for (const value of ['invalid', 'null', '[null]', '[{label: Guide}]', '[{label: "", url: /workflows}]']) {
    await assert.rejects(fixture(t, `---\nrelated_docs: ${value}\n---\n# Example\n`).loadContent(),
      /example\/README\.md: related_docs must be a list of nonempty label\/url pairs/);
  }
});

test('the GitOps publishing example teaches built-in Git commands directly', () => {
  for (const relative of [
    '../../../examples/gitops/README.md',
    '../../../examples/gitops/atmos.yaml',
  ]) {
    const source = readFileSync(new URL(relative, import.meta.url), 'utf8');
    assert.doesNotMatch(source, /atmos gitops|name:\s*["']?gitops\b/);
    for (const command of ['clone', 'status', 'diff', 'clean']) {
      assert.match(source, new RegExp(`(?:atmos )?git ${command} deploy`));
    }
  }
});
