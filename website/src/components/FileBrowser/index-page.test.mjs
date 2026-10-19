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


function loadCatalog(react, router, isBrowser = true, playerType = () => null, runtime = {}) {
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
  vm.runInNewContext(catalogCode, {
    ...runtime,
    module: catalogModule,
    exports: catalogModule.exports,
    require(name) {
      if (name === 'react') return react;
      if (name === '@docusaurus/router') return router;
      if (name === '@docusaurus/useIsBrowser') return () => isBrowser;
      if (name === '@docusaurus/useIsomorphicLayoutEffect') return runtime.useLayoutEffect || (() => {});
      if (name === '../CastPlayer') return playerType;
      if (name === 'react-icons/ri') return new Proxy({}, { get: () => () => null });
      if (name.endsWith('.module.css')) return { __esModule: true, default: {} };
      return passthrough;
    },
  }, { filename: catalogFilename });
  return catalogModule.exports.default;
}

function catalogPlayer({ paused = false, hidden = false, reducedMotion = false, inViewport = true } = {}) {
  // Model a loaded, visible preview and the user's current page preferences.
  const states = ['', paused, reducedMotion, hidden, inViewport, true];
  const playerType = () => null;
  const Catalog = loadCatalog({
    ...React,
    useState() {
      assert.ok(states.length > 0, 'unexpected catalog state');
      return [states.shift(), () => {}];
    },
    useRef: () => ({ current: null }),
    useEffect: () => {},
  }, {
    useLocation: () => ({ pathname: '/examples', search: '', hash: '' }),
    useHistory: () => ({ push() {} }),
  }, true, playerType);
  const example = { name: 'demo', title: 'Demo', tags: [], cast: { file: 'demo.cast' }, root: { children: [] } };
  const page = Catalog({
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

function catalogNavigation({ hash = '', search = '', isBrowser = true } = {}) {
  const locations = [{ pathname: '/examples', search, hash }];
  let cursor = 0;
  let query = '';
  const Catalog = loadCatalog({
    ...React,
    useState(initial) {
      return typeof initial === 'string'
        ? [query, (next) => { query = next; }]
        : [initial, () => {}];
    },
    useRef: () => ({ current: null }),
    useEffect: () => {},
  }, {
    useLocation: () => locations[cursor],
    useHistory: () => ({ push(next) {
      locations.splice(cursor + 1, Infinity, next);
      cursor++;
    } }),
  }, isBrowser);
  const examples = [
    ['aws', ['Emulators']],
    ['backend', ['Terraform', 'Emulators']],
    ['functions', ['YAML Functions']],
    ['quickstart', ['Quickstart']],
  ].map(([name, tags]) => ({ name, title: name, tags, root: { children: [] } }));
  function render() {
    const page = Catalog({
      tree: { featured: [examples[0]], examples, tags: ['Emulators', 'Terraform', 'YAML Functions', 'Quickstart'] },
      options: { routeBasePath: '/examples', title: 'Examples' },
    });
    const elements = [];
    function visit(element) {
      if (!React.isValidElement(element)) return;
      elements.push(element);
      React.Children.forEach(element.props.children, visit);
    }
    visit(page);
    return {
      selected: elements.find((el) => el.type === 'select').props.value,
      cards: elements.filter((el) => el.props.example).map((el) => el.props.example.name),
      buttons: (tag) => elements.filter((el) => el.type === 'button' && el.key === tag),
      select: (tag) => elements.find((el) => el.type === 'select').props.onChange({ target: { value: tag } }),
      search: (value) => elements.find((el) => el.type === 'input').props.onChange({ target: { value } }),
      clear: () => elements.find((el) => el.type === 'button' && React.Children.toArray(el.props.children).includes('Clear filters ')).props.onClick(),
    };
  }
  return { render, locations, location: () => locations[cursor], back: () => cursor--, forward: () => cursor++ };
}

test('chapter fragments select matching examples, including secondary tags and multiword chapters', () => {
  for (const [hash, category, cards] of [
    ['#emulators', 'Emulators', ['aws', 'backend']],
    ['#yaml-functions', 'YAML Functions', ['functions']],
    ['#quickstart', 'Quickstart', ['quickstart']],
    ['#featured', 'Featured', ['aws']],
    ['', 'All', ['aws', 'backend', 'functions', 'quickstart']],
    ['#unknown', 'All', ['aws', 'backend', 'functions', 'quickstart']],
    ['#%invalid', 'All', ['aws', 'backend', 'functions', 'quickstart']],
  ]) {
    const page = catalogNavigation({ hash }).render();
    assert.equal(page.selected, category);
    assert.deepEqual(page.cards, cards);
    assert.ok(page.buttons(category).every((button) => button.props['aria-pressed']));
  }
});

test('static rendering ignores the fragment until hydration completes', () => {
  const page = catalogNavigation({ hash: '#emulators', isBrowser: false }).render();
  assert.equal(page.selected, 'All');
  assert.deepEqual(page.cards, ['aws', 'backend', 'functions', 'quickstart']);
});

test('sidebar, chips, and dropdown update history while preserving query parameters', () => {
  const catalog = catalogNavigation({ search: '?layout=gallery&utm_source=test' });
  catalog.render().buttons('Emulators')[0].props.onClick();
  assert.equal(catalog.location().hash, '#emulators');
  catalog.render().buttons('Emulators')[0].props.onClick();
  assert.equal(catalog.locations.length, 2, 'reselecting a chapter must not add history');
  catalog.render().buttons('YAML Functions')[1].props.onClick();
  assert.equal(catalog.location().hash, '#yaml-functions');
  catalog.render().select('Featured');
  assert.equal(catalog.location().hash, '#featured');
  assert.equal(catalog.location().pathname, '/examples');
  assert.equal(catalog.location().search, '?layout=gallery&utm_source=test');
  catalog.back();
  assert.equal(catalog.render().selected, 'YAML Functions');
  catalog.back();
  assert.deepEqual(catalog.render().cards, ['aws', 'backend']);
  catalog.forward();
  assert.equal(catalog.render().selected, 'YAML Functions');
  catalog.render().select('All');
  assert.equal(catalog.location().hash, '');
  assert.equal(catalog.render().selected, 'All');
});

test('clear filters removes the chapter fragment and resets search', () => {
  const catalog = catalogNavigation({ hash: '#emulators', search: '?utm_source=test' });
  catalog.render().search('backend');
  assert.deepEqual(catalog.render().cards, ['backend']);
  catalog.render().clear();
  assert.equal(catalog.location().hash, '');
  assert.equal(catalog.location().search, '?utm_source=test');
  assert.deepEqual(catalog.render().cards, ['aws', 'backend', 'functions', 'quickstart']);
});

test('chapter navigation restores scroll immediately after the router scrolls to the top', () => {
  let effect;
  let frame;
  let restored;
  const scrollRef = { current: null };
  const location = { pathname: '/examples', search: '', hash: '#emulators' };
  const window = { scrollX: 0, scrollY: 250, scrollTo: (options) => { restored = options; } };
  const Catalog = loadCatalog({
    ...React,
    useState: (initial) => [initial, () => {}],
    useRef: () => scrollRef,
    useEffect: () => {},
  }, {
    useLocation: () => location,
    useHistory: () => ({ push() {} }),
  }, true, () => null, {
    window,
    useLayoutEffect: (callback) => { effect = callback; },
    requestAnimationFrame: (callback) => { frame = callback; return 1; },
    cancelAnimationFrame: () => {},
  });
  const props = { tree: { featured: [], examples: [], tags: ['Emulators'] }, options: {} };
  Catalog(props);
  const cleanup = effect();
  assert.equal(frame, undefined, 'initial navigation must keep native scroll behavior');
  cleanup();
  location.hash = '';
  Catalog(props);
  effect();
  window.scrollY = 0;
  frame();
  assert.equal(restored, undefined, 'wait until the router scroll has started');
  frame();
  assert.equal(restored.top, 250);
  assert.equal(restored.left, 0);
  assert.equal(restored.behavior, 'instant', 'restoration must cancel the router smooth scroll');
});
