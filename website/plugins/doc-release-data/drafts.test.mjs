import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import plugin from './index.js';

test('draft pages are absent from both release lookups and the unreleased index', async (t) => {
  const root = mkdtempSync(path.join(tmpdir(), 'atmos-draft-docs-'));
  const previousDirectory = process.cwd();
  t.after(() => {
    process.chdir(previousDirectory);
    rmSync(root, { recursive: true, force: true });
  });
  // An empty repository makes every visible fixture unreleased without network access.
  execFileSync('git', ['init', '-q', root]);
  process.chdir(root);
  mkdirSync(path.join(root, 'docs'));
  const pages = {
    draft: 'draft: true\ntitle: Hidden\nslug: /hidden',
    published: 'draft: false\ntitle: Published\nslug: /published-route',
    ordinary: 'title: Ordinary',
  };
  for (const [name, frontmatter] of Object.entries(pages)) {
    writeFileSync(path.join(root, 'docs', `${name}.mdx`), `---\n${frontmatter}\n---\n# Content\n`);
  }

  const result = await plugin({ siteDir: root }, {}).loadContent();

  assert.deepEqual(result.releaseMap, {
    '/ordinary': 'unreleased',
    '/ordinary/': 'unreleased',
    '/published': 'unreleased',
    '/published/': 'unreleased',
  });
  assert.deepEqual(result.unreleasedDocs, [
    { path: '/ordinary', title: 'Ordinary', description: null },
    { path: '/published-route', title: 'Published', description: null },
  ]);
});
