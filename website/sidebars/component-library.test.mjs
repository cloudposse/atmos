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
sidebar_group: component-library
sidebar_label: ${type}
sidebar_position: 1
component_type: ${type}
component_implementation: Example implementation
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
    'elsewhere/second.md': '---\nid: stable-id\nsidebar_label: Second\nsidebar_position: 2\nsidebar_group: component-library\nsidebar_parent: guides/engine\n---\n',
    'elsewhere/first.mdx': '---\ntitle: First\nsidebar_position: 1\nsidebar_group: component-library\nsidebar_parent: guides/engine\n---\n',
    'stack/shared.mdx': overview('shared', 'sidebar_reference: true\nslug: /existing/url\n'),
    'custom.mdx': '---\ntitle: custom\nsidebar_group: component-library\n---\n',
  });
  const items = componentLibraryItems(root);
  const engine = items.find(item => item.label === 'engine');
  assert.equal(engine.link.id, 'guides/engine');
  assert.deepEqual(engine.items.map(item => item.id), ['elsewhere/first', 'elsewhere/stable-id']);
  const shared = items.find(item => item.label === 'shared');
  assert.equal(shared.type, 'link');
  assert.equal(shared.href, '/existing/url');
  assert.equal(shared.customProps.navigationReference, true);
  assert.equal(items.find(item => item.label === 'custom').customProps.componentType, undefined);
});

test('invalid, duplicate, and orphaned metadata fails with the source document', t => {
  for (const [files, error] of [
    [{'bad.mdx': '---\ntitle: Bad\nsidebar_group: component-library\ncomponent_type: true\n---\n'}, /bad.mdx: component_type must be a nonempty string/],
    [{'a.mdx': overview('tool'), 'b.mdx': overview('tool')}, /b.mdx: duplicate component type tool/],
    [{'orphan.mdx': '---\ntitle: Orphan\nsidebar_group: component-library\nsidebar_parent: missing\n---\n'}, /orphan.mdx: unknown sidebar_parent missing/],
    [{'shared.mdx': overview('shared', 'sidebar_reference: true\n')}, /shared.mdx: shared documents require an absolute slug/],
  ]) assert.throws(() => componentLibraryItems(fixture(t, files)), error);
});

test('the repository index includes all native overviews and preserves Terraform guides', () => {
  const items = componentLibraryItems();
  validateSidebars({components: items});
  assert.deepEqual(items.filter(item => item.customProps.componentType?.native).map(item => item.customProps.componentType.id),
    ['terraform', 'kubernetes', 'helm', 'helmfile', 'packer', 'ansible', 'container', 'aws/cloudformation', 'emulator']);
  assert.deepEqual(items[0].items.map(item => item.label),
    ['Stack Configuration', 'Root Modules', 'State Backends', 'Workspaces', 'Provider Generation', 'Planfiles', 'Brownfield']);
  assert.equal(items.some(item => item.label === 'metadata' || item.label === 'provision'), false);
});
