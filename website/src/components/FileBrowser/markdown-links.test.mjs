import assert from 'node:assert/strict';
import test from 'node:test';
import { resolveMarkdownLink } from './markdown-links.mjs';

for (const [href, file, base, expected] of [
  ['stacks/catalog/app.yaml', 'cloudformation-advanced/README.md', '/examples', '/examples/cloudformation-advanced/stacks/catalog/app.yaml'],
  ['./scripts/test.py#main', 'cloudformation-advanced/README.md', '/examples', '/examples/cloudformation-advanced/scripts/test.py#main'],
  ['../cloudformation/', 'cloudformation-advanced/README.md', '/examples', '/examples/cloudformation/'],
  ['../config.yaml?raw=true#section', 'demo/docs/guide.md', '/examples/', '/examples/demo/config.yaml?raw=true#section'],
  ['references/usage.md', 'demo/SKILL.md', '/skills', '/skills/demo/references/usage.md'],
  ['a%20file.yaml', 'demo/README.md', '/examples', '/examples/demo/a%20file.yaml'],
  ['config.yaml', 'a # directory/README.md', '/examples', '/examples/a%20%23%20directory/config.yaml'],
  ['#setup', 'demo/README.md', '/examples', '#setup'],
  ['?view=source', 'demo/README.md', '/examples', '?view=source'],
  ['/steps/publish', 'demo/README.md', '/examples', '/steps/publish'],
  ['https://atmos.tools/', 'demo/README.md', '/examples', 'https://atmos.tools/'],
  ['//example.com/path', 'demo/README.md', '/examples', '//example.com/path'],
  ['mailto:team@example.com', 'demo/README.md', '/examples', 'mailto:team@example.com'],
  ['', 'demo/README.md', '/examples', ''],
]) {
  test(`${file}: ${href}`, () => assert.equal(resolveMarkdownLink(href, file, base), expected));
}

test('repository-only documents link to their GitHub source', () => {
  assert.equal(
    resolveMarkdownLink('../../docs/prd/atmos-profiles.md#design', 'config-profiles/README.md', '/examples', 'https://github.com/cloudposse/atmos/blob/main/examples/config-profiles/README.md'),
    'https://github.com/cloudposse/atmos/blob/main/docs/prd/atmos-profiles.md#design',
  );
});

test('published sibling examples remain on the website', () => {
  assert.equal(
    resolveMarkdownLink('../cloudformation/', 'cloudformation-advanced/README.md', '/examples', 'https://github.com/cloudposse/atmos/blob/main/examples/cloudformation-advanced/README.md'),
    '/examples/cloudformation/',
  );
});
