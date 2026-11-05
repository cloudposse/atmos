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

const unreleased = 'unreleased';

// Each post is stored under the one permalink Docusaurus generates, with and without a slash.
function expectedEntries(permalink, release = unreleased) {
  return { [permalink]: release, [`${permalink}/`]: release };
}

test('posts are keyed by their actual Docusaurus permalink and README.md is skipped', async (t) => {
  const root = mkdtempSync(path.join(tmpdir(), 'atmos-blog-release-'));
  const previousDirectory = process.cwd();
  t.after(() => {
    process.chdir(previousDirectory);
    rmSync(root, { recursive: true, force: true });
  });
  // An empty repository makes every post unreleased without network access.
  execFileSync('git', ['init', '-q', root]);
  process.chdir(root);
  // A dated file without a slug lives at its date path, not at a slug-only alias.
  writePost(root, ['2026-01-01-top-level.mdx'], 'title: Top');
  // Same slug as a later dated-folder post: neither may overwrite the other.
  writePost(root, ['2026-01-02-slugged.mdx'], 'title: Slugged\nslug: feature');
  writePost(root, ['2026', '01', '03', 'feature.mdx'], 'title: Feature');
  // Dates are found inside nested paths, keeping the folder in the permalink.
  writePost(root, ['nested', '2026-01-04-feature.mdx'], 'title: Nested dated');
  // A leading slash in the frontmatter slug is normalized away.
  writePost(root, ['nested', '2026-01-05-leading-slash.mdx'], 'title: Leading\nslug: /leading');
  writePost(root, ['nested', 'plain-post.md'], 'title: Plain');
  writePost(root, ['nested', 'overridden.mdx'], 'title: Override\nrelease: v1.2.3');
  writePost(root, ['README.md'], 'title: Readme');

  const { releaseMap } = await plugin({ siteDir: root }, {}).loadContent();

  assert.deepEqual(releaseMap, {
    ...expectedEntries('/changelog/2026/01/01/top-level'),
    ...expectedEntries('/changelog/feature'),
    ...expectedEntries('/changelog/2026/01/03/feature'),
    ...expectedEntries('/changelog/2026/01/04/nested/feature'),
    ...expectedEntries('/changelog/leading'),
    ...expectedEntries('/changelog/nested/plain-post'),
    ...expectedEntries('/changelog/nested/overridden', 'v1.2.3'),
  });
});
