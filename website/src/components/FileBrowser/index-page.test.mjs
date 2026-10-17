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
let requestedQuery = '';
const reactWithQuery = {
  ...React,
  useState(initial) {
    const [value, update] = React.useState(initial);
    return [typeof initial === 'string' ? requestedQuery : value, update];
  },
};
const passthrough = ({ children }) => React.createElement(React.Fragment, null, children);
vm.runInNewContext(code, {
  module,
  exports: module.exports,
  require(name) {
    if (name === 'react') return reactWithQuery;
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

function renderSections(examples, tags, query = '') {
  requestedQuery = query;
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


test('search results appear once even when the match carries a secondary tag', () => {
  assert.deepEqual(renderSections([
    ['generate-files', ['Terraform', 'Automation']],
    ['workflow', ['Automation']],
  ], ['Terraform', 'Automation'], 'generate-files'), [
    { tag: 'Terraform', names: ['generate-files'] },
  ]);
});


function catalogPlayer({ paused = false, hidden = false, reducedMotion = false, inViewport = true } = {}) {
  const catalogFilename = fileURLToPath(new URL('./ExampleCatalogPage.tsx', import.meta.url));
  const { code: catalogCode } = transformSync(readFileSync(catalogFilename, 'utf8'), {
    filename: catalogFilename,
    babelrc: false,
    configFile: false,
    presets: [
      babelRequire.resolve('@babel/preset-typescript'),
      [babelRequire.resolve('@babel/preset-react'), { runtime: 'classic' }],
      [babelRequire.resolve('@babel/preset-env'), { targets: { node: 'current' }, modules: 'commonjs' }],
    ],
  });
  const catalogModule = { exports: {} };
  // Model a loaded, visible preview and the user's current page preferences.
  const states = ['All', '', paused, reducedMotion, hidden, inViewport, true];
  const playerType = () => null;
  const reactWithPreferences = {
    ...React,
    useState() {
      assert.ok(states.length > 0, 'unexpected catalog state');
      return [states.shift(), () => {}];
    },
    useRef: () => ({ current: null }),
    useEffect: () => {},
  };
  vm.runInNewContext(catalogCode, {
    module: catalogModule,
    exports: catalogModule.exports,
    require(name) {
      if (name === 'react') return reactWithPreferences;
      if (name === '@docusaurus/router') return { useLocation: () => ({ search: '' }) };
      if (name === '@docusaurus/useIsBrowser') return () => true;
      if (name === '../CastPlayer') return playerType;
      if (name === 'react-icons/ri') return new Proxy({}, { get: () => () => null });
      if (name.endsWith('.module.css')) return { __esModule: true, default: {} };
      return passthrough;
    },
  }, { filename: catalogFilename });
  const example = { name: 'demo', title: 'Demo', tags: [], cast: { file: 'demo.cast' }, root: { children: [] } };
  const page = catalogModule.exports.default({
    tree: { featured: [], examples: [example], tags: [] },
    options: { title: 'Examples', description: 'Demos' },
  });
  function findPlayer(element) {
    if (!React.isValidElement(element)) return null;
    if (element.type === playerType) return element;
    if (typeof element.type === 'function') return findPlayer(element.type(element.props));
    for (const child of React.Children.toArray(element.props.children)) {
      const found = findPlayer(child);
      if (found) return found;
    }
    return null;
  }
  const player = findPlayer(page);
  assert.ok(player, 'catalog must render its loaded cast preview');
  assert.equal(states.length, 0, 'all page and preview preferences must be consumed');
  return player;
}

test('pausing or hiding a catalog preview preserves its player identity and playback mode', () => {
  const playing = catalogPlayer();
  assert.equal(playing.props.paused, false);
  assert.equal(playing.props.static, false);
  for (const state of [{ paused: true }, { hidden: true }, { inViewport: false }]) {
    const stopped = catalogPlayer(state);
    assert.equal(stopped.key, playing.key, 'pausing must not remount the player');
    assert.equal(stopped.props.static, false, 'pausing must not jump to the final frame');
    assert.equal(stopped.props.paused, true);
  }
  const reduced = catalogPlayer({ reducedMotion: true });
  assert.equal(reduced.props.static, true, 'reduced motion must retain still previews');
  assert.notEqual(reduced.key, playing.key);
});
