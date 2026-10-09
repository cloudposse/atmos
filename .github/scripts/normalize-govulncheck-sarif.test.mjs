import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

import { deduplicateStacks } from './normalize-govulncheck-sarif.mjs';

const stack = {
  message: { text: 'A call stack for vulnerable function example.com/module.Run' },
  frames: [{ location: { physicalLocation: { region: { startLine: 12 } } } }],
};

function reportWith(stacks) {
  return { version: '2.1.0', runs: [{ tool: { driver: { name: 'govulncheck' } }, results: [{ ruleId: 'GO-2026-0001', stacks }] }] };
}

test('removes structurally identical stacks, preserving distinct traces and order', () => {
  const otherLine = structuredClone(stack);
  otherLine.frames[0].location.physicalLocation.region.startLine = 13;
  const otherMessage = { ...stack, message: { text: 'Another vulnerable function' } };
  const reordered = { frames: structuredClone(stack.frames), message: structuredClone(stack.message) };
  const report = reportWith([stack, otherLine, reordered, otherMessage, structuredClone(stack)]);
  assert.equal(deduplicateStacks(report), 2);
  assert.deepEqual(report, reportWith([stack, otherLine, otherMessage]));
  assert.equal(deduplicateStacks(report), 0);
});

test('keeps findings in every run, including shared stacks and absent stack arrays', () => {
  const report = reportWith([stack, structuredClone(stack)]);
  report.runs[0].results.push({ ruleId: 'GO-2026-0002', stacks: [stack] }, { ruleId: 'GO-2026-0003' });
  report.runs.push({ tool: { driver: { name: 'another-run' } } });
  report.runs.push({ results: [{ ruleId: 'GO-2026-0004', stacks: [] }] });
  const expected = structuredClone(report);
  expected.runs[0].results[0].stacks = [stack];
  assert.equal(deduplicateStacks(report), 1);
  assert.deepEqual(report, expected);
});

test('leaves invalid stack values for the existing SARIF validation gate to reject', () => {
  const report = reportWith({ invalid: true });
  const expected = structuredClone(report);
  assert.equal(deduplicateStacks(report), 0);
  assert.deepEqual(report, expected);
});

test('CLI writes a normalized copy and fails on malformed JSON', t => {
  const dir = mkdtempSync(join(tmpdir(), 'govulncheck-sarif-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const input = join(dir, 'input.sarif');
  const output = join(dir, 'output.sarif');
  const script = fileURLToPath(new URL('./normalize-govulncheck-sarif.mjs', import.meta.url));
  const original = JSON.stringify(reportWith([stack, stack]));
  writeFileSync(input, original);
  const run = spawnSync(process.execPath, [script, input, output], { encoding: 'utf8' });
  assert.equal(run.status, 0, run.stderr);
  assert.deepEqual(JSON.parse(readFileSync(output, 'utf8')), reportWith([stack]));
  assert.equal(readFileSync(input, 'utf8'), original);
  writeFileSync(input, '{broken');
  const failed = spawnSync(process.execPath, [script, input, output], { encoding: 'utf8' });
  assert.notEqual(failed.status, 0);
});
