import assert from 'node:assert/strict';
import {readFileSync, readdirSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import test from 'node:test';
import matter from 'gray-matter';
import plugin from './index.js';
import {policyPage, policySidebar} from './policy-pages.js';
import {inputFields} from './policy-input-fields.js';
import {effectLabel, outcomeLabel, policySidebarLinks, policyViewUrl, resolvePolicyView} from '../../src/components/PolicyLibrary/policy-view.mjs';
import {findSection, filterItems} from '../../src/components/SidebarNavigator/navigation.mjs';

const siteDir = fileURLToPath(new URL('../../', import.meta.url));
const libraryDir = path.resolve(siteDir, '../policy-library');

test('the policy gallery discovers individually documented and tested policies', async () => {
  const browser = plugin({siteDir}, {
    id: 'policies',
    sourceDir: '../policy-library',
    githubRepo: 'cloudposse/atmos',
    githubPath: 'policy-library',
    routeBasePath: '/policies/library',
    title: 'Policy Library',
    tagOrder: ['Terraform', 'GitHub Actions', 'Lifecycle Hooks'],
    searchable: true,
    cardCtaLabel: 'View policy',
    enablePerPageMarkdown: true,
    presentation: 'policies',
    docsSidebar: 'cli',
  });
  const content = await browser.loadContent();
  const entries = readdirSync(libraryDir, {withFileTypes: true}).filter(entry => entry.isDirectory());
  assert.ok(entries.length > 0, 'The library must not be empty');
  assert.deepEqual(content.tree.examples.map(entry => entry.name).sort(), entries.map(entry => entry.name).sort());
  assert.deepEqual(content.tree.tags, ['Terraform', 'GitHub Actions', 'Lifecycle Hooks', 'Admission', 'Affected Changes']);
  assert.equal(content.options.searchable, true);
  assert.equal(content.options.installCommandTemplate, '', 'Do not invent a policy install command');
  assert.equal(content.options.enableCopyMarkdown, false, 'Keep the first-pass gallery controls minimal');

  const routes = [];
  const modules = new Map();
  await browser.contentLoaded({content, actions: {
    createData: async (name, content) => { modules.set(`data/${name}`, JSON.parse(content)); return `data/${name}`; },
    addRoute: route => routes.push(route),
    setGlobalData: () => {},
  }});
  const paths = new Set(routes.map(route => route.path));
  assert.equal(paths.size, routes.length, 'Generated routes must be unique');
  assert.ok(paths.has('/policies/library'));
  assert.match(routes[0].component, /PolicyLibrary\/IndexPage/);
  assert.equal(modules.get(routes[0].modules.treeData).examples[0].root, undefined, 'Index must not bundle every manifest and test');
  const configuration = readFileSync(path.join(siteDir, 'docs/policies/configuration.mdx'), 'utf8');
  assert.match(configuration, /^## Enforcement$/m);

  for (const entry of content.tree.examples) {
    const base = `/policies/library/${entry.name}`;
    const route = routes.find(route => route.path === base);
    assert.match(route.component, /PolicyLibrary\/PolicyPage/);
    const page = modules.get(route.modules.policyData);
    assert.equal(route.modules.treeData, undefined, 'Policy navigation uses the shared sidebar, not a second catalog in the page');
    const summary = modules.get(routes[0].modules.treeData).examples.find(item => item.name === entry.name);
    const manifest = matter.engines.yaml.parse(readFileSync(path.join(libraryDir, entry.name, 'policy.yaml'), 'utf8'));
    const rule = Object.values(manifest.spec.policies)[0];
    assert.equal(page.action, rule.action);
    assert.equal(page.inputKind, rule.on);
    assert.equal(summary.action, rule.action);
    assert.equal(summary.inputKind, rule.on);
    assert.deepEqual(page.variants.map(variant => variant.engine), ['cel', 'starlark']);
    for (const variant of page.variants) {
      assert.equal(variant.policy, readFileSync(path.join(libraryDir, entry.name, variant.path), 'utf8'));
      assert.ok(paths.has(`${base}/${variant.path}`), `Missing variant route: ${base}/${variant.path}`);
      assert.ok(variant.githubUrl.endsWith(`/${entry.name}/${variant.path}`));
      const variantRule = Object.values(matter.engines.yaml.parse(variant.policy).spec.policies)[0];
      assert.equal(variant.reason, variantRule.reason?.trim() || undefined);
    }
    assert.deepEqual(page.cases, JSON.parse(readFileSync(path.join(libraryDir, entry.name, 'tests/cases.json'), 'utf8')));
    assert.deepEqual(page.caseFields, page.cases.map(fixture => inputFields(fixture.request)));
    assert.ok(page.configuration && page.contract);
    assert.match(page.command, /^atmos \S/);
    for (const suffix of ['', '/policy.yaml', '/tests/cases.json']) {
      assert.ok(paths.has(base + suffix), `Missing catalog route: ${base + suffix}`);
    }
    const {data, content: readme} = matter(entry.root.readme.content);
    assert.deepEqual(page.caseLabels, data.scenarios ?? {});
    for (const name of Object.keys(page.caseLabels)) {
      assert.ok(page.cases.some(fixture => fixture.name === name), `Unknown scenario label: ${entry.name}/${name}`);
    }
    assert.equal(page.command, data.command, 'Use the documented command, including repository-wide commands');
    assert.ok(data.title && data.description && data.tags.length, `${entry.name}: catalog metadata required`);
    assert.ok(entry.docs.length >= 2, `${entry.name}: reference links required`);
    for (const section of ['Contract', 'Configuration', 'Policy', 'Use', 'Tests']) {
      assert.ok(readme.includes(`## ${section}\n`), `${entry.name}: missing ${section}`);
    }
    const links = [...readme.matchAll(/\]\(([^)]+)\)/g)].map(match => match[1]);
    for (const target of [`${base}/policy.yaml`, `${base}/tests/cases.json`, '/policies/configuration#enforcement']) {
      assert.ok(links.includes(`https://atmos.tools${target}`), `${entry.name}: missing documentation link ${target}`);
    }
    for (const link of links) {
      // READMEs also render on GitHub and are checked outside Docusaurus by Lychee.
      // Root-relative website routes are interpreted as missing repository files there.
      assert.ok(!link.startsWith('/'), `${entry.name}: use an absolute documentation URL: ${link}`);
      if (link.startsWith('https://atmos.tools/policies/library/')) {
        const url = new URL(link);
        assert.ok(paths.has(url.pathname), `${entry.name}: broken library link ${link}`);
      }
    }
    const fixtures = JSON.parse(readFileSync(path.join(libraryDir, entry.name, 'tests/cases.json'), 'utf8'));
    assert.ok(fixtures.some(fixture => ['deny', 'allow', 'awaiting_approval'].includes(fixture.status)), `${entry.name}: matching case required`);
    assert.ok(fixtures.some(fixture => fixture.status === 'not_applicable'), `${entry.name}: nonmatching case required`);
  }
});

test('policy links round-trip engine, view and named cases without losing unrelated parameters', () => {
  const cases = [{name: 'missing Owner / unknown?'}, {name: 'owned'}];
  const url = policyViewUrl('/policies/library/owner', '?source=docs', {
    view: 'examples', engine: 'starlark', case: cases[0].name,
  });
  const parsed = new URL(url, 'https://atmos.tools');
  assert.equal(parsed.searchParams.get('source'), 'docs');
  assert.deepEqual(resolvePolicyView(parsed.search, '', cases, 'cel'), {
    view: 'examples', engine: 'starlark', fixture: cases[0],
  });
  const reference = policyViewUrl(parsed.pathname, parsed.search, {view: 'reference'});
  assert.equal(new URL(reference, parsed).searchParams.get('case'), cases[0].name);
  const otherPolicy = new URL(policyViewUrl('/policies/library/approval', parsed.search, {case: null}), parsed);
  assert.equal(otherPolicy.pathname, '/policies/library/approval');
  assert.equal(otherPolicy.searchParams.has('case'), false);
  assert.equal(otherPolicy.searchParams.get('view'), 'examples');
  assert.equal(otherPolicy.searchParams.get('engine'), 'starlark');
  assert.equal(resolvePolicyView('', '', cases, 'starlark').engine, 'starlark');
  assert.deepEqual(resolvePolicyView('?view=bad&engine=bad&case=bad', '', cases, 'starlark'), {
    view: 'policy', engine: 'starlark', fixture: cases[0],
  });
  assert.equal(resolvePolicyView('', '#examples', cases).view, 'examples');
  assert.equal(resolvePolicyView('?view=reference', '#examples', cases).view, 'reference');
  assert.deepEqual(resolvePolicyView('', '', [], 'unknown'), {view: 'policy', engine: 'cel', fixture: undefined});
});

test('decision labels distinguish nonmatching, allowed, blocked, approval and hook outcomes', () => {
  assert.equal(effectLabel('deny'), 'Blocks');
  assert.equal(effectLabel('require_approval'), 'Requires approval');
  assert.equal(effectLabel('hooks'), 'Requires hooks');
  assert.equal(outcomeLabel({status: 'not_applicable'}), 'Rule does not match');
  assert.equal(outcomeLabel({status: 'allow'}), 'Allowed');
  assert.equal(outcomeLabel({status: 'allow', hooks: [{}]}), 'Hooks required');
  assert.equal(outcomeLabel({status: 'deny', hooks: [{}]}), 'Blocked');
  assert.equal(outcomeLabel({status: 'awaiting_approval', hooks: [{}]}), 'Approval required');
  assert.equal(outcomeLabel({status: 'future-status'}), 'future-status');
});

test('library sidebar focuses policy titles, preserves guide navigation, and carries view selections', () => {
  const original = [
    {type: 'category', label: 'Commands', href: '/cli', items: []},
    {type: 'category', label: 'Policies', href: '/policies', customProps: {navigationSidebar: 'cli'}, items: [
      {type: 'link', label: 'Library', href: '/policies/library'},
      {type: 'link', label: 'Configuration', href: '/policies/configuration'},
    ]},
  ];
  const before = structuredClone(original);
  const examples = [{name: 'owner', title: 'Require an Owner tag'}, {name: 'review', title: 'Require approval'}];
  const sidebar = policySidebar(original, examples, '/policies/library');
  assert.deepEqual(original, before, 'Ordinary documentation must retain its sidebar');
  assert.equal(sidebar[0], original[0]);
  assert.equal(sidebar[1].label, 'Policy Library');
  assert.equal(sidebar[1].customProps.navigationSidebar, 'cli', 'Mobile and desktop share sidebar identity');
  assert.equal(sidebar[1].items[0].href, '/policies');
  assert.deepEqual(sidebar[1].items.slice(1).map(item => item.label), examples.map(item => item.title));
  for (const path of ['/policies/library', '/policies/library/owner', '/policies/library/owner/policy.yaml']) {
    assert.equal(findSection(sidebar, path), 1, 'Catalog, policy, and source pages focus the library');
  }
  assert.equal(filterItems(sidebar, 'owner')[0].items[0].href, '/policies/library/owner');
  const selected = policySidebarLinks(sidebar, '?case=absent&source=docs', {engine: 'starlark', view: 'examples'});
  assert.equal(selected[1].items[0].href, '/policies', 'Guide links must not acquire policy parameters');
  assert.equal(sidebar[1].items[1].href, '/policies/library/owner', 'URL decoration must not mutate shared data');
  for (const item of selected[1].items.slice(1)) {
    const url = new URL(item.href, 'https://atmos.tools');
    assert.equal(url.searchParams.get('case'), null);
    assert.equal(url.searchParams.get('engine'), 'starlark');
    assert.equal(url.searchParams.get('view'), 'examples');
    assert.equal(url.searchParams.get('source'), 'docs');
  }
});

test('field explorer describes observed fields without inventing a provider schema', () => {
  const request = {input: {items: [null, true, 1, 'value', [], {}], 'odd.key"\n': 'private'}, ci: {event: {data: {pull_request: {head: {repo: {full_name: 'contributor/repo'}}}}}}};
  const fields = inputFields(request);
  for (const [path, type] of [
    ['input', 'object'],
    ['input["items"]', 'array'],
    ['input["items"][0]', 'null'],
    ['input["items"][1]', 'boolean'],
    ['input["items"][2]', 'number'],
    ['input["items"][3]', 'string'],
    ['input["items"][4]', 'array'],
    ['input["items"][5]', 'object'],
    ['input["odd.key\\"\\n"]', 'string'],
    ['ci["event"]["data"]["pull_request"]["head"]["repo"]["full_name"]', 'string'],
  ]) {
    assert.ok(fields.some(field => field.path === path && field.type === type), path);
  }
  assert.ok(!JSON.stringify(fields).includes('contributor/repo'));
  assert.deepEqual(inputFields({input: {}}), [{path: 'input', type: 'object'}, {path: 'ci', type: 'null'}]);
  assert.deepEqual(inputFields({input: {z: 1, a: 2}}), inputFields({input: {a: 2, z: 1}}));
  assert.ok(!fields.some(field => field.path.includes('metadata')), 'Absent fields stay absent');
  assert.ok(inputFields({kind: 'operation', input: {}, subject: {labels: {stage: 'prod'}}})
    .some(field => field.path === 'request["subject"]["labels"]["stage"]' && field.type === 'string'));
});

test('documentation navigation uses resolved docs links and fails on a missing sidebar', async () => {
  const browser = plugin({siteDir}, {id: 'policies', docsSidebar: 'cli'});
  const content = {tree: {examples: []}, options: {id: 'policies'}};
  let global;
  const actions = {setGlobalData: value => {global = value;}};
  const allContent = {'file-browser': {policies: content}, 'docusaurus-plugin-content-docs': {default: {loadedVersions: [{
    versionName: 'current',
    sidebars: {cli: [{type: 'doc', id: 'policies/index'}]},
    docs: [{id: 'policies/index', title: 'Policies', permalink: '/policies', frontMatter: {}}],
  }]}}};
  await browser.allContentLoaded({allContent, actions});
  assert.equal(global.sidebar.name, 'cli');
  assert.equal(global.sidebar.items[0].href, '/policies');
  await assert.rejects(browser.allContentLoaded({allContent: {'file-browser': {policies: content}}, actions}), /sidebar not found/);
  await plugin({siteDir}, {}).allContentLoaded({allContent: {}, actions: {setGlobalData: () => assert.fail('Standalone galleries must not acquire docs navigation')}});
});

test('incomplete policy pages fail the build rather than publishing missing code', () => {
  assert.throws(() => policyPage({name: 'missing', root: {children: []}}), /Incomplete policy/);
});

function variantExample() {
  const name = 'function-disable-exec';
  return {name, root: {
    readme: {content: readFileSync(path.join(libraryDir, name, 'README.md'), 'utf8')},
    children: [
      {name: 'policy.yaml', content: readFileSync(path.join(libraryDir, name, 'policy.yaml'), 'utf8')},
      {name: 'tests', children: [{name: 'cases.json', content: '[]'}]},
      {name: 'variants', children: [{name: 'starlark', children: [
        {name: 'policy.yaml', content: readFileSync(path.join(libraryDir, name, 'variants/starlark/policy.yaml'), 'utf8')},
      ]}]},
    ],
  }};
}

test('variant tabs reject missing, duplicated, unsupported, or mislabeled implementations', () => {
  const missing = variantExample();
  missing.root.children.pop();
  assert.throws(() => policyPage(missing), /exactly one CEL and one Starlark/);

  const duplicate = variantExample();
  duplicate.root.children[2].children.push(duplicate.root.children[2].children[0]);
  assert.throws(() => policyPage(duplicate), /exactly one CEL and one Starlark/);

  const unsupported = variantExample();
  unsupported.root.children[2].children[0].name = 'opa';
  assert.throws(() => policyPage(unsupported), /Invalid policy variant/);

  const mislabeled = variantExample();
  mislabeled.root.children[2].children[0].children[0].content = mislabeled.root.children[0].content;
  assert.throws(() => policyPage(mislabeled), /Invalid policy engine/);
});
