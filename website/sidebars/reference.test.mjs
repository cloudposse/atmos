import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import sidebars from '../sidebars.js';
import ci from './ci.js';
import {canonicalSidebar} from '../src/components/SidebarNavigator/canonical.mjs';
import {filterItems, findSection, prepareItems} from '../src/components/SidebarNavigator/navigation.mjs';

test('Reference exposes CLI essentials and capabilities without extra category levels', () => {
  assert.equal(sidebars.cli[0].id, 'reference-overview');
  assert.equal(sidebars.cli[0].customProps.navigationOverview, true);
  const groups = sidebars.cli.filter(item => item.customProps?.navigationGroup);
  assert.deepEqual(groups.map(item => item.customProps.navigationGroup), [
    'CLI essentials', 'Capabilities', 'Configuration', 'Automation', 'Guides & resources',
  ]);
  assert.ok(groups.every(item => item.type === 'html'));
  assert.deepEqual(sidebars.cli.slice(2, 6).map(item => item.label), [
    'Commands', 'Global Flags', 'Environment Variables', 'Versioning',
  ]);
  assert.deepEqual(sidebars.cli.slice(7, 11).map(item => item.label), [
    'CI/CD', 'Atmos AI & MCP', 'Atmos Pro', 'Editor Integration',
  ]);
  assert.equal(sidebars.cli.some(item => ['CLI', 'GitHub Actions', 'Stack Guides'].includes(item.label)), false);
});

test('workflow name placeholder links to its definition', () => {
  const workflow = sidebars.cli.find(item => item.label === 'Workflows');
  const named = workflow.items[0].items[0];
  assert.equal(named.label, '<name>');
  assert.deepEqual(named.link, {type: 'doc', id: 'workflows/workflows/workflow/name'});
});

test('CI/CD exposes workflow guides and orders capabilities from setup to review', () => {
  const github = ci.items[0];
  assert.equal(github.link.id, 'integrations/github-actions/index');
  assert.equal(prepareItems([github], '/ci')[0].collapsed, false);
  assert.deepEqual(github.items.map(item => item.label), [
    'Setup Atmos', 'Plan on Pull Request', 'Apply on Merge', 'Deploy Affected Components',
    'Deploy All Components', 'Deployment Approvals', 'Authentication & Permissions', 'Validate Workflows',
  ]);
  assert.deepEqual(ci.items.slice(1).map(item => item.label), [
    'CI Configuration', 'Planfile Storage', 'Build Cache', 'Outputs', 'Job Summaries',
    'Pull Request Comments', 'Status Checks', 'Migrate from Legacy Actions',
  ]);
  for (const item of github.items) {
    const content = readFileSync(new URL(`../docs/${item.id}.mdx`, import.meta.url), 'utf8');
    assert.match(content, /<Intro>/, `${item.id} needs an introduction`);
  }
});

test('CI filtering crosses visual groups and shared configuration retains its canonical section', () => {
  const resolvedCI = {...ci, href: '/ci', items: ci.items.map(item =>
    item.type === 'category' ? {...item, href: '/integrations/github-actions', items: item.items.map(doc => ({
      ...doc, type: 'link', docId: doc.id, href: `/${doc.id}`,
    }))} : item)};
  const configuration = {type: 'category', label: 'CLI Configuration', items: [
    {type: 'link', docId: 'cli/configuration/ci/checks', label: 'checks', href: '/cli/configuration/ci/checks'},
  ]};
  const items = [sidebars.cli[1], resolvedCI, configuration];
  assert.equal(filterItems(items, 'ci/cd plan on pull request')[0].items[0].items[0].label, 'Plan on Pull Request');
  assert.equal(findSection(items, '/cli/configuration/ci/checks'), 2);
  const breadcrumbs = canonicalSidebar(items);
  assert.equal(breadcrumbs[1].items.some(item => item.href === '/cli/configuration/ci/checks'), false);
  assert.equal(breadcrumbs[2].items[0].href, '/cli/configuration/ci/checks');
  assert.equal(findSection(items, '/integrations/github-actions/plan-on-pull-request'), 1);
});

test('Native CI retains historical workflow anchors with links to focused guides', () => {
  const source = readFileSync(new URL('../docs/ci/ci.mdx', import.meta.url), 'utf8');
  for (const heading of [
    'GitHub Actions Workflows', 'Plan on Pull Request', 'Apply on Merge', 'Deploy Affected',
    'Deploy All', 'Gating Production with Environments', 'Workflow Setup and Reference',
    'Permissions', 'Authentication', 'Caching the Toolchain', 'SBOM Artifacts', 'Validate Workflows',
  ]) assert.ok(source.includes(` ${heading}\n`), `Preserve the ${heading} fragment`);
  assert.match(source, /\/integrations\/github-actions\/plan-on-pull-request/);
  assert.match(source, /\/integrations\/github-actions\/authentication#permissions/);
});

test('the workflow name placeholder opens its naming guide', () => {
  const workflows = sidebars.cli.find(item => item.label === 'Workflows');
  const name = workflows.items[0].items.find(item => item.label === '<name>');
  assert.equal(name.link.id, 'workflows/name');
  assert.ok(name.items.some(item => item.label === 'steps'));
});
