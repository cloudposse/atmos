const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { test } = require('node:test');

// Run the script by its name from its own directory, so no absolute path reaches the command line.
const script = 'govulncheck-monitor.sh';
const options = { skip: process.platform === 'win32' };

for (const status of [0, 23]) {
  test(`preserves scanner output and exit status ${status}, and stops the monitor`, options, () => {
    const result = spawnSync('bash', [script, 'bash', '-c',
      `sleep 0.2; printf '%s' '{"version":"2.1.0"}'; printf 'scanner diagnostic\\n' >&2; exit ${status}`],
    { cwd: __dirname, encoding: 'utf8', timeout: 5000 });
    assert.ifError(result.error);
    assert.equal(result.status, status);
    assert.equal(result.stdout, '{"version":"2.1.0"}');
    assert.match(result.stderr, /govulncheck resources/);
    assert.match(result.stderr, /scanner diagnostic/);
  });
}
