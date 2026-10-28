import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import plugin from './index.js';

function writePost(root, segments, frontmatter) {
  const file = path.join(root, 'blog', ...segments);
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, `---\n${frontmatter}\n---\n# Content\n`);
}

test('nested posts are included in the release map and README.md is skipped', async (t) => {
  const root = mkdtempSync(path.join(tmpdir(), 'atmos-blog-release-'));
  const previousDirectory = process.cwd();
  t.after(() => {
    process.chdir(previousDirectory);
    rmSync(root, { recursive: true, force: true });
  });
  // An empty repository makes every post unreleased without network access.
  execFileSync('git', ['init', '-q', root]);
  process.chdir(root);
  writePost(root, ['2026-01-01-top-level.mdx'], 'title: Top');
  writePost(root, ['2026', '01', '03', 'dated-folders.mdx'], 'title: Dated folders');
  writePost(root, ['nested', 'plain-post.md'], 'title: Plain');
  writePost(root, ['nested', 'overridden.mdx'], 'title: Override\nrelease: v1.2.3');
  writePost(root, ['README.md'], 'title: Readme');

  const { releaseMap } = await plugin({ siteDir: root }, {}).loadContent();

  assert.deepEqual(releaseMap, {
    '/changelog/top-level': 'unreleased',
    '/changelog/top-level/': 'unreleased',
    '/changelog/2026/01/01/top-level': 'unreleased',
    '/changelog/2026/01/01/top-level/': 'unreleased',
    '/changelog/dated-folders': 'unreleased',
    '/changelog/dated-folders/': 'unreleased',
    '/changelog/2026/01/03/dated-folders': 'unreleased',
    '/changelog/2026/01/03/dated-folders/': 'unreleased',
    '/changelog/nested/plain-post': 'unreleased',
    '/changelog/nested/plain-post/': 'unreleased',
    '/changelog/nested/overridden': 'v1.2.3',
    '/changelog/nested/overridden/': 'v1.2.3',
  });
});
