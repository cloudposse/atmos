import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { isDeepStrictEqual } from 'node:util';

// SARIF requires result.stacks to contain unique objects. govulncheck can emit
// identical call stacks for a finding; retain the first copy in its original order.
// Leave every result and every distinct stack intact for code-scanning validation.
export function deduplicateStacks(report) {
  let removed = 0;
  for (const run of report.runs) {
    for (const result of run.results ?? []) {
      if (!Array.isArray(result.stacks)) continue;
      const unique = [];
      for (const stack of result.stacks) {
        if (unique.some(existing => isDeepStrictEqual(existing, stack))) {
          removed++;
        } else {
          unique.push(stack);
        }
      }
      result.stacks = unique;
    }
  }
  return removed;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [input, output] = process.argv.slice(2);
  if (!input || !output) throw new Error('Usage: normalize-govulncheck-sarif.mjs INPUT OUTPUT');
  const report = JSON.parse(readFileSync(input, 'utf8'));
  const removed = deduplicateStacks(report);
  writeFileSync(output, JSON.stringify(report, null, 2) + '\n');
  console.log(`Removed ${removed} duplicate SARIF call stacks; all findings retained.`);
}
