import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const require = createRequire(import.meta.url);
// Resolve the compiler through Docusaurus so this works with pnpm's isolated dependencies.
const docusaurusRequire = createRequire(require.resolve('@docusaurus/core/package.json'));
const babelRequire = createRequire(docusaurusRequire.resolve('@docusaurus/babel'));
const { transformSync } = babelRequire('@babel/core');
const filename = fileURLToPath(new URL('./IndexPage.tsx', import.meta.url));
const { code } = transformSync(readFileSync(filename, 'utf8'), {
  filename,
  babelrc: false,
  configFile: false,
  sourceMaps: 'inline',
  presets: [
    babelRequire.resolve('@babel/preset-typescript'),
    [babelRequire.resolve('@babel/preset-react'), { runtime: 'classic' }],
    [babelRequire.resolve('@babel/preset-env'), { targets: { node: 'current' }, modules: 'commonjs' }],
  ],
});
const module = { exports: {} };
const passthrough = ({ children }) => React.createElement(React.Fragment, null, children);
vm.runInNewContext(code, {
  module,
  exports: module.exports,
  require(name) {
    if (name === 'react') return React;
    if (name === '@theme/Layout' || name === 'react-markdown') return passthrough;
    if (name === '@docusaurus/Link') {
      return ({ to, children, ...props }) => React.createElement('a', { href: to, ...props }, children);
    }
    if (name === 'remark-gfm') return () => {};
    if (name === '@fortawesome/react-fontawesome') return { FontAwesomeIcon: () => null };
    if (name === '@fortawesome/free-solid-svg-icons') return { faFolder: {}, faGraduationCap: {} };
    if (name.endsWith('.module.css')) return { __esModule: true, default: new Proxy({}, { get: (_, key) => key }) };
    // Media/copy controls are unrelated to the index's section membership.
    if (name.startsWith('./') || name.startsWith('@site/')) return () => null;
    throw new Error(`Unexpected component dependency: ${name}`);
  },
}, { filename });
const IndexPage = module.exports.default;

function renderSections(examples, tags) {
  const html = renderToStaticMarkup(React.createElement(IndexPage, {
    treeData: { examples: examples.map(([name, tags]) => ({ name, title: name, tags, root: {} })), tags },
    optionsData: { routeBasePath: '/gists', title: 'Projects', description: 'Project examples' },
  }));
  return [...html.matchAll(/<section class="tagSection"><h2 class="tagSectionHeading">([^<]+)<\/h2>(.*?)<\/section>/g)]
    .map(([, tag, body]) => ({
      tag,
      names: [...body.matchAll(/<h2 class="exampleCardTitle">([^<]+)<\/h2>/g)].map((match) => match[1]),
    }));
}

test('secondary-only tags retain a visible section, while empty tags disappear', () => {
  assert.deepEqual(renderSections([
    ['emulated', ['Emulators', 'Terraform']],
    ['untagged', []],
  ], ['Emulators', 'Terraform', 'Unused']), [
    { tag: 'Emulators', names: ['emulated'] },
    { tag: 'Terraform', names: ['emulated'] },
    { tag: 'More', names: ['untagged'] },
  ]);
});

test('primary examples take precedence over secondary-tag fallback', () => {
  assert.deepEqual(renderSections([
    ['emulated', ['Emulators', 'Terraform']],
    ['native', ['Terraform']],
  ], ['Terraform', 'Emulators']), [
    { tag: 'Terraform', names: ['native'] },
    { tag: 'Emulators', names: ['emulated'] },
  ]);
});
