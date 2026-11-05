import assert from 'node:assert/strict';
import {mkdtempSync, mkdirSync, writeFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import test from 'node:test';
import {sidebarGroupItems} from './sidebar-group.js';

function fixture(t, files) {
  const root = mkdtempSync(path.join(tmpdir(), 'sidebar-group-'));
  t.after(() => rmSync(root, {recursive: true, force: true}));
  for (const [name, metadata] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(root, name)), {recursive: true});
    writeFileSync(path.join(root, name), `---\n${metadata}\n---\n`);
  }
  return root;
}

test('groups reuse existing label, order, styling, and description across folders', t => {
  const docsDir = fixture(t, {
    'a.mdx': 'title: A\nsidebar_group: editors',
    'z.mdx': 'title: Z\nsidebar_group: editors',
    'elsewhere/tool.mdx': 'title: Long title\nsidebar_label: Tool\nsidebar_group: editors\nsidebar_position: 1\nsidebar_class_name: command\ndescription: Editor support',
    'guide.mdx': 'title: Setup\nsidebar_group: editors\nsidebar_parent: elsewhere/tool',
    'other.mdx': 'title: Excluded\nsidebar_group: ci',
  });
  const items = sidebarGroupItems('editors', {docsDir});
  assert.deepEqual(items.map(item => item.label), ['Tool', 'A', 'Z']);
  assert.equal(items[0].className, 'command');
  assert.equal(items[0].description, 'Editor support');
  assert.deepEqual(items[0].link, {type: 'doc', id: 'elsewhere/tool'});
  assert.equal(items[0].items[0].id, 'guide');
  assert.equal(sidebarGroupItems('ci', {docsDir})[0].id, 'other');
});

test('invalid ancestry fails even when the cycle has no root entry', t => {
  for (const [files, error] of [
    [{'a.mdx': 'title: A\nsidebar_group: test\nsidebar_parent: b', 'b.mdx': 'title: B\nsidebar_group: test\nsidebar_parent: a'}, /cyclic sidebar_parent/],
    [{'a.mdx': 'title: A\nsidebar_group: test\nsidebar_parent: b', 'b.mdx': 'title: B\nsidebar_group: other'}, /unknown sidebar_parent b in test/],
    [{'a.mdx': 'title: A\nsidebar_group: test\nsidebar_reference: true\nslug: /shared', 'b.mdx': 'title: B\nsidebar_group: test\nsidebar_parent: a'}, /shared documents cannot own child guides/],
  ]) assert.throws(() => sidebarGroupItems('test', {docsDir: fixture(t, files)}), error);
});

test('invalid ordering, duplicate document IDs, and unsafe reference URLs are rejected', t => {
  for (const [files, error] of [
    [{'a.mdx': 'title: A\nsidebar_group: test\nsidebar_position: first'}, /sidebar_position must be a number/],
    [{'a.mdx': 'title: A\nsidebar_group: test', 'b.mdx': 'id: a\ntitle: B\nsidebar_group: test'}, /duplicate document ID a/],
    [{'a.mdx': 'title: A\nsidebar_group: test\nsidebar_reference: true\nslug: //example.com'}, /require an absolute slug/],
  ]) assert.throws(() => sidebarGroupItems('test', {docsDir: fixture(t, files)}), error);
});
