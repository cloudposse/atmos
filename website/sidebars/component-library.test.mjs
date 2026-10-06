import assert from 'node:assert/strict';
import {mkdtempSync, mkdirSync, writeFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import test from 'node:test';
import {createRequire} from 'node:module';
const require = createRequire(import.meta.url);
const {validateSidebars} = require('../node_modules/@docusaurus/plugin-content-docs/lib/sidebars/validation.js');
import {componentLibraryItems} from './component-library.js';

function fixture(t, files) {
  const root = mkdtempSync(path.join(tmpdir(), 'component-library-'));
  t.after(() => rmSync(root, {recursive: true, force: true}));
  for (const [name, contents] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(root, name)), {recursive: true});
    writeFileSync(path.join(root, name), contents);
  }
  return root;
}
const overview = (type, extra = '') => `---
title: ${type}
component_library:
  type: ${type}
  label: ${type}
  position: 1
  implementation: Example implementation
  description: Example purpose
${extra}---
`;

test('a new overview anywhere in the docs automatically feeds navigation and overview metadata', t => {
  const root = fixture(t, {
    'new/tool.mdx': overview('new-tool'),
    'stacks/fields.mdx': '---\ntitle: Fields\n---\nConfiguration reference',
  });
  const items = componentLibraryItems(root);
  assert.equal(items.length, 1);
  assert.equal(items[0].id, 'new/tool');
  assert.deepEqual(items[0].customProps.componentType, {
    id: 'new-tool', native: true, implementation: 'Example implementation', description: 'Example purpose',
  });
  validateSidebars({components: items});
});

test('shared overviews retain canonical routes and nested guides use their own metadata', t => {
  const root = fixture(t, {
    'guides/engine.mdx': overview('engine'),
    'elsewhere/second.md': '---\nid: stable-id\nsidebar_label: Second\nsidebar_position: 2\ncomponent_library_parent: engine\n---\n',
    'elsewhere/first.mdx': '---\ntitle: First\nsidebar_position: 1\ncomponent_library_parent: engine\n---\n',
    'stack/shared.mdx': overview('shared', '  reference: true\nslug: /existing/url\n'),
    'custom.mdx': overview('custom', '  native: false\n'),
  });
  const items = componentLibraryItems(root);
  const engine = items.find(item => item.label === 'engine');
  assert.equal(engine.link.id, 'guides/engine');
  assert.deepEqual(engine.items.map(item => item.id), ['elsewhere/first', 'elsewhere/stable-id']);
  const shared = items.find(item => item.label === 'shared');
  assert.equal(shared.type, 'link');
  assert.equal(shared.href, '/existing/url');
  assert.equal(shared.customProps.navigationReference, true);
  assert.equal(items.find(item => item.label === 'custom').customProps.componentType.native, false);
});

test('invalid, duplicate, and orphaned metadata fails with the source document', t => {
  for (const [files, error] of [
    [{'bad.mdx': '---\ncomponent_library: true\n---\n'}, /bad.mdx: component_library must be an object/],
    [{'a.mdx': overview('tool'), 'b.mdx': overview('tool')}, /b.mdx: duplicate component type tool/],
    [{'orphan.mdx': '---\ncomponent_library_parent: missing\n---\n'}, /orphan.mdx: unknown component_library_parent missing/],
    [{'shared.mdx': overview('shared', '  reference: true\n')}, /shared.mdx: shared component overviews require an absolute slug/],
  ]) assert.throws(() => componentLibraryItems(fixture(t, files)), error);
});

test('the repository index includes all native overviews and preserves Terraform guides', () => {
  const items = componentLibraryItems();
  validateSidebars({components: items});
  assert.deepEqual(items.filter(item => item.customProps.componentType.native).map(item => item.customProps.componentType.id),
    ['terraform', 'kubernetes', 'helm', 'helmfile', 'packer', 'ansible', 'container', 'emulator']);
  assert.deepEqual(items[0].items.map(item => item.label),
    ['Stack Configuration', 'Root Modules', 'State Backends', 'Workspaces', 'Provider Generation', 'Planfiles', 'Brownfield']);
  assert.equal(items.some(item => item.label === 'metadata' || item.label === 'provision'), false);
});
