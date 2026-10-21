const assert = require('node:assert/strict');
const { test } = require('node:test');
const filter = require('./filter-semgrep-sarif.cjs');

test('removes inline suppressions but retains an unpinned external action for the same rule', () => {
  const ruleId = 'yaml.github-actions.security.github-actions-mutable-action-tag.github-actions-mutable-action-tag';
  const external = { ruleId, message: { text: 'uses: actions/checkout@v6' }, partialFingerprints: { hash: 'abc' } };
  const report = { version: '2.1.0', runs: [{ results: [
    { ruleId, message: { text: 'uses: $/.github/actions/release-publish' }, suppressions: [{ kind: 'inSource' }] },
    external,
  ] }] };
  assert.deepEqual(filter(report).runs[0].results, [external]);
});

test('retains pending, rejected, external, empty and absent suppressions', () => {
  const results = [
    { suppressions: [{ kind: 'inSource', status: 'underReview' }] },
    { suppressions: [{ kind: 'inSource', status: 'rejected' }] },
    { suppressions: [{ kind: 'external', status: 'accepted' }] },
    { suppressions: [] },
    { ruleId: 'another-real-finding' },
  ];
  assert.deepEqual(filter({ runs: [{ results: structuredClone(results) }] }).runs[0].results, results);
});

test('filters every run while preserving SARIF metadata and empty runs', () => {
  const metadata = { tool: { driver: { name: 'Semgrep', rules: [{ id: 'rule' }] } }, artifacts: [{ location: { uri: 'test.yml' } }] };
  const report = { version: '2.1.0', $schema: 'sarif-schema', runs: [
    { ...metadata, results: [{ suppressions: [{ kind: 'inSource', status: 'accepted' }] }] },
    { results: [{ suppressions: [{ kind: 'inSource' }] }] },
    { ...metadata },
  ] };
  assert.deepEqual(filter(report), { version: '2.1.0', $schema: 'sarif-schema', runs: [
    { ...metadata, results: [] }, { results: [] }, { ...metadata },
  ] });
});

test('malformed reports fail instead of silently producing an empty upload', () => {
  for (const report of [{}, { runs: null }, { runs: [{ results: {} }] }]) {
    assert.throws(() => filter(report), TypeError);
  }
});
