import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const require = createRequire(import.meta.url);
const coreRequire = createRequire(require.resolve('@docusaurus/core/package.json'));
const babelRequire = createRequire(coreRequire.resolve('@docusaurus/babel'));
const filename = fileURLToPath(new URL('./index.tsx', import.meta.url));
const { code } = babelRequire('@babel/core').transformSync(readFileSync(filename, 'utf8'), {
  filename,
  babelrc: false,
  configFile: false,
  presets: [
    babelRequire.resolve('@babel/preset-typescript'),
    [babelRequire.resolve('@babel/preset-react'), { runtime: 'classic' }],
    [babelRequire.resolve('@babel/preset-env'), { targets: { node: 'current' }, modules: 'commonjs' }],
  ],
});
const h = React.createElement;
const module = { exports: {} };
vm.runInNewContext(code, {
  module,
  exports: module.exports,
  require(name) {
    if (name === 'react') return React;
    if (name === '@docusaurus/Link') return ({ to, children }) => h('a', { href: to }, children);
    if (name === '@theme/Tabs') return ({ defaultValue, children }) => h('div', { 'data-default': defaultValue }, children);
    if (name === '@theme/TabItem') return ({ label, value, children }) => h('section', { 'data-label': label, 'data-value': value }, children);
    if (name === '@theme/CodeBlock') return ({ children }) => h('pre', null, children);
    if (name.endsWith('/InstallWidget')) return () => h('aside', null, 'INSTALL_WIDGET');
    if (name.endsWith('.css')) return {};
    throw new Error(`Unexpected dependency: ${name}`);
  },
}, { filename });
const { default: TryExample, TryExamples } = module.exports;

test('single example starts with the example and preserves follow-up instructions', () => {
  const html = renderToStaticMarkup(h(TryExample, { example: 'policies' }, 'Run offline fixtures.'));
  assert.match(html, /data-default="example:policies"/);
  assert.match(html, /data-label="Try the example"/);
  assert.match(html, /atmos init examples\/policies\ncd policies/);
  assert.match(html, /Run offline fixtures\./);
  assert.match(html, /href="\/examples\/policies"/);
  assert.match(html, /href="\/cli\/commands\/init#examples-and-source-directories"/);
});

test('group shares installation and keeps each named example and instructions distinct', () => {
  const html = renderToStaticMarkup(h(TryExamples, null,
    h(TryExample, { example: 'devcontainer', label: 'Dev Container' }, 'List containers.'),
    h(TryExample, { example: 'devcontainer-build', label: 'Build Geodesic' }, 'Build Geodesic locally.'),
  ));
  assert.equal(html.match(/INSTALL_WIDGET/g).length, 1);
  assert.equal(html.match(/data-label="Install Atmos"/g).length, 1);
  assert.match(html, /data-default="example:devcontainer"/);
  assert.match(html, /data-label="Dev Container"/);
  assert.match(html, /data-label="Build Geodesic"/);
  assert.match(html, /atmos init examples\/devcontainer\ncd devcontainer/);
  assert.match(html, /atmos init examples\/devcontainer-build\ncd devcontainer-build/);
  assert.match(html, /List containers\./);
  assert.match(html, /Build Geodesic locally\./);
});

test('detail-page variant omits self-links and uses the final path segment for cd', () => {
  const html = renderToStaticMarkup(h(TryExample, {
    example: 'nested/project',
    showExampleLink: false,
  }, h('a', { href: '#readme' }, 'README')));
  assert.match(html, /atmos init examples\/nested\/project\ncd project/);
  assert.doesNotMatch(html, /href="\/examples\//);
  assert.match(html, /href="#readme"/);
  assert.match(html, /href="\/install"/);
});

test('unlabeled grouped examples remain identifiable and an empty group renders nothing', () => {
  const html = renderToStaticMarkup(h(TryExamples, null,
    h(TryExample, { example: 'one' }),
    h(TryExample, { example: 'two' }),
  ));
  assert.match(html, /data-label="one"/);
  assert.match(html, /data-label="two"/);
  assert.equal(renderToStaticMarkup(h(TryExamples, null)), '');
});
