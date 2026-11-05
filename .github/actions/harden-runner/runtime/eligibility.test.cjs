const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { parse } = require('./yaml.cjs');

// Exercise the checked-in composite's wiring, not a copy of its condition.
// This condition uses property access, string equality, and boolean operators,
// which have the same semantics in JavaScript for these string-valued inputs.
const action = parse(fs.readFileSync(path.join(__dirname, '..', 'action.yml'), 'utf8'));
const helper = action.runs.steps.find(step => step.uses === '$/.github/actions/harden-runner/runtime');
assert.ok(helper, 'the composite must invoke the DNS repair runtime');
const expression = helper.with['dns-repair'];
assert.match(expression, /^\$\{\{.*\}\}$/);
const condition = new vm.Script(expression.slice(3, -2).replaceAll('inputs.dns-repair', "inputs['dns-repair']"));

for (const arch of ['X64', 'ARM64']) {
  for (const [name, os, environment, requested, expected] of [
    ['hosted Linux default', 'Linux', 'github-hosted', action.inputs['dns-repair'].default, true],
    ['explicit opt-out', 'Linux', 'github-hosted', 'false', false],
    ['self-hosted Linux', 'Linux', 'self-hosted', 'true', false],
    ['hosted Windows', 'Windows', 'github-hosted', 'true', false],
    ['hosted macOS', 'macOS', 'github-hosted', 'true', false],
  ]) {
    test(`${name} on ${arch} forwards DNS repair=${expected}`, () => {
      const actual = condition.runInNewContext({
        inputs: { 'dns-repair': requested },
        runner: { os, arch, environment },
      }, { timeout: 1000 });
      assert.equal(actual, expected);
    });
  }
}
