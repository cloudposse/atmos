const { test } = require('node:test');
const assert = require('node:assert/strict');
const { render } = require('./summary.cjs');

function environment(overrides = {}) {
  return {
    GITHUB_SERVER_URL: 'https://github.com', GITHUB_REPOSITORY: 'cloudposse/atmos',
    GITHUB_RUN_ID: '39985198501', GITHUB_RUN_ATTEMPT: '2',
    RELEASE_STAGE: 'prepare', RELEASE_STATUS: 'success', RELEASE_MODE: 'candidate',
    RELEASE_VERSION: 'v1.233.2', RELEASE_TAG: 'v1.233.2-rc.3', RELEASE_COMMIT: 'a'.repeat(40),
    RELEASE_MILESTONE: '13', RELEASE_ARTIFACT: '123',
    RELEASE_CHECKS: JSON.stringify([{ name: 'Reserve tag', result: 'success' }]),
    ...overrides,
  };
}

test('successful preparation shows the reserved RC, source, milestone and artifact links', () => {
  const summary = render(environment());
  assert.match(summary, /v1\.233\.2-rc\.3/);
  assert.doesNotMatch(summary, /rc\.39985198501/);
  assert.match(summary, /commit\/a{40}/);
  assert.match(summary, /milestone\/13/);
  assert.match(summary, /actions\/runs\/39985198501\/artifacts\/123/);
  assert.match(summary, /Run 39985198501, attempt 2/);
  assert.match(summary, /Reserve tag \| ✅ Success/);
  assert.match(summary, /tag reserved/);
  assert.match(summary, /Prereleases leave the stable milestone open/);
  assert.doesNotMatch(summary, /releases\/tag/);
});

test('credential failure shows requested version and actionable configuration without claiming reservation or publication', () => {
  const summary = render(environment({
    RELEASE_STATUS: 'failure', RELEASE_TAG: '', RELEASE_ARTIFACT: '',
    RELEASE_CHECKS: JSON.stringify([
      { name: 'Credentials', result: 'failure' }, { name: 'Reserve tag', result: 'skipped' },
    ]),
  }));
  assert.match(summary, /v1\.233\.2/);
  assert.match(summary, /Not reserved/);
  assert.match(summary, /Failed checkpoint: \*\*Credentials\*\*/);
  assert.match(summary, /Reserve tag \| ⏭ Skipped/);
  assert.match(summary, /BOT_GITHUB_APP_PRIVATE_KEY/);
  assert.match(summary, /explicitly forward `BOT_GITHUB_APP_PRIVATE_KEY`/);
  assert.doesNotMatch(summary, /Download artifacts|tag reserved|release published|releases\/tag/);
});

test('all stages describe their own successful work and only publication links the release', () => {
  for (const [stage, text] of [
    ['build', 'Artifacts built, signed, attested, and saved.'],
    ['verify', 'Artifact identities, signatures, and provenance verified'],
    ['publish', 'Immutable release published and verified.'],
    ['complete', 'Stable milestone bookkeeping completed.'],
  ]) {
    const summary = render(environment({ RELEASE_STAGE: stage, RELEASE_MODE: 'cut' }));
    assert.ok(summary.includes(text));
    assert.equal(summary.includes('/releases/tag/v1.233.2-rc.3'), stage === 'publish');
    assert.doesNotMatch(summary, /Prereleases/);
  }
});

test('failed and cancelled stages retain reservation and never claim publication success', () => {
  for (const status of ['failure', 'cancelled']) {
    const summary = render(environment({
      RELEASE_STAGE: 'publish', RELEASE_STATUS: status,
      RELEASE_CHECKS: JSON.stringify([{ name: 'Publish', result: status }]),
    }));
    assert.match(summary, /Retry this run to retain its reserved tag/);
    assert.match(summary, /job logs and annotations/);
    assert.doesNotMatch(summary, /Immutable release published|releases\/tag/);
    assert.match(summary, status === 'cancelled' ? /⏹ Cancelled/ : /❌ Failed/);
  }
});

test('nightly and feature summaries work without optional milestone or requested version', () => {
  for (const mode of ['nightly', 'feature']) {
    const summary = render(environment({
      RELEASE_MODE: mode, RELEASE_VERSION: '', RELEASE_TAG: '', RELEASE_COMMIT: '',
      RELEASE_MILESTONE: '', RELEASE_ARTIFACT: '', RELEASE_STATUS: 'cancelled',
      RELEASE_CHECKS: '[]',
    }));
    assert.ok(summary.includes(`<code>${mode}</code>`));
    assert.match(summary, /This stage did not complete/);
    assert.doesNotMatch(summary, /Requested version|Source commit|Milestone \||Artifacts \||Retry this run/);
  }
});

test('unrecognized status and checkpoints do not imply success', () => {
  const summary = render(environment({
    RELEASE_STATUS: '', RELEASE_CHECKS: '[{"name":"Preparation","result":""}]',
  }));
  assert.match(summary, /Result \| Unknown/);
  assert.match(summary, /Preparation \| Not run/);
  assert.match(summary, /This stage did not complete/);
});

test('escapes untrusted version, source, milestone, tag, and checkpoint text', () => {
  for (const tag of ['script', 'SCRIPT', 'ScRiPt']) {
    const payload = `<${tag}>alert("x")</${tag}>'&|\r\n`;
    const summary = render(environment({
      RELEASE_TAG: payload, RELEASE_COMMIT: payload, RELEASE_MILESTONE: payload,
      RELEASE_VERSION: payload, RELEASE_CHECKS: JSON.stringify([{ name: payload, result: 'success' }]),
    }));
    assert.ok(!summary.toLowerCase().includes('<script'));
    assert.ok(!summary.toLowerCase().includes('</script'));
    assert.ok(summary.includes(`&lt;${tag}&gt;`));
    assert.match(summary, /&quot;x&quot;/);
    assert.match(summary, /&#39;&amp;&#124;  /);
    assert.ok(summary.includes(`%3C${tag}%3E`));
  }
});

test('rejects invalid stages and malformed checkpoint data', () => {
  assert.throws(() => render(environment({ RELEASE_STAGE: 'invalid' })), /Unknown release summary stage/);
  assert.throws(() => render(environment({ RELEASE_CHECKS: '{' })), SyntaxError);
});
