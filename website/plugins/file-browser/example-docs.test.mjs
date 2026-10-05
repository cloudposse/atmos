import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import plugin from './index.js';

const siteDir = fileURLToPath(new URL('../../', import.meta.url));
const normalize = (url) => url.replace(/\/$/, '');

test('every published example links to its command and configuration documentation', async () => {
  const { tree } = await plugin({ siteDir }, { sourceDir: '../examples' }).loadContent();
  assert.ok(tree.examples.length > 0);
  for (const example of tree.examples) {
    assert.ok(example.docs.length >= 2, `${example.name}: add relevant documentation links`);
    assert.ok(example.root.readme, `${example.name}: add a README explaining the example`);
    const urls = example.docs.map(({ url }) => normalize(url));
    assert.equal(new Set(urls).size, urls.length, `${example.name}: duplicate documentation links`);
    const readmeLinks = [...example.root.readme.content.matchAll(/https:\/\/atmos\.tools(\/[^\s)>]*)/g)]
      .map((match) => normalize(match[1]));
    for (const { label, url } of example.docs) {
      assert.ok(label.trim(), `${example.name}: documentation link needs a label`);
      assert.ok(url.startsWith('/') && !url.startsWith('//'), `${example.name}: use a local documentation route`);
      assert.ok(readmeLinks.includes(normalize(url)), `${example.name}: README missing ${url}`);
    }
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
