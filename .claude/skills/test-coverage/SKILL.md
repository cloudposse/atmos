---
name: test-coverage
description: "Run tests scoped to the current patch vs origin/main, fix any failures found in those packages (this patch's own regressions or genuinely pre-existing ones alike), then fix genuine coverage gaps on added lines — never coverage theater. Invoke on explicit requests like \"check test coverage\" / \"are my tests passing\", or from within the fix-all skill's cycle."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Test Coverage (patch-scoped)

Answers two questions about the current branch's patch vs `origin/main`: are its tests passing,
and is it adequately covered? Both come from the same scoped test run, so one skill owns both.

## Why patch-scoped, not full-suite

The full suite is 349 packages / 1876 test files and takes 45-75 minutes per CI timeouts — running
it every hour is not feasible. This skill only ever *runs* tests for packages containing files this
patch touched — it never expands to the other 340+ packages just to go looking for trouble. But
once a package is in scope and its run turns up a failure, fix it regardless of whether that
specific test or file is what this patch changed: what this loop cares about is the packages it
touched actually passing, not whose commit originally broke them.

## Running the check

```
atmos fix coverage [base-ref]
```

(`atmos fix tests [base-ref]` is an equivalent alias — same underlying run, reach for it when the
question framing is "are my tests passing" rather than "is my patch covered.") Defaults to
`origin/main`. This wraps `.claude/skills/test-coverage/scripts/patch-test-coverage.sh`, which
deliberately does no line-level analysis itself — it just prints a raw bundle (pass/fail status,
raw test output if failed, the coverage profile if passed, and the diff) for the fixing agent to
reason over directly, the same way `coderabbit-review` reads raw CodeRabbit markdown rather than
us pre-parsing it.

Three outcomes:

- **`STATUS: NO_GO_CHANGES`** — no touched `.go` files. One-line no-op, done.
- **`STATUS: TESTS_FAILING`** — go to "Fix failing tests" below.
- **`STATUS: OK`** — tests pass; go to "Fix coverage gaps" below.

## Fix failing tests (gates coverage — a package with failing tests has meaningless coverage
numbers)

Delegate to `Agent subagent_type: "test-coverage-fix"`, Section A, passing the raw failing-test
output and the list of touched files. The agent classifies each failure as **in-scope** (the file
lives among this patch's touched files) or **pre-existing** (it doesn't), but scope only changes
how much diagnosis a fix needs — not whether an attempt is made. What this loop cares about is the
suite passing, full stop; "not this patch's fault" is not a reason to leave something red.
Confirmed for real: a "pre-existing failure" was itself a false positive in the *check*, not the
code — the patch-scoped `go test` run had no `-timeout` override, so Go's 10-minute binary default
tripped on the full (patch-unrelated) `./tests` acceptance package under load, panicking on
whichever subtest happened to still be running when the clock ran out. The fix was to the
test-infrastructure itself (`patch-test-coverage.sh` now passes `-timeout 40m`, matching
`.atmos.d/test.yaml`'s full-suite convention) — exactly the kind of pre-existing-but-fixable root
cause this agent should resolve rather than just report.

**Non-negotiable, equal severity to the anti-theater rule below:** never weaken or delete an
assertion just to force a red test green. If the test's own expectation was wrong (not the code),
fix the test to assert the *correct* behavior and say so explicitly — don't just make it pass.

One fix attempt per failing test per cycle, in-scope or pre-existing alike. Still red after that,
or the agent isn't confident it can safely attempt a fix at all (e.g. it would require touching a
hard-prohibited file, or needs a credential/decision only a human has): stop, report for human
attention, and invoke the [`say` skill](../say/SKILL.md) with something like `"PR <number>
coverage check needs your input."` Don't loop indefinitely inside an hourly budget.

Only proceed to coverage gaps when every scoped test passes. If any failure remains red after its
single fix attempt, report it, invoke `say`, and stop; do not analyze coverage from a failing test
run.

## Fix coverage gaps

Delegate to `Agent subagent_type: "test-coverage-fix"`, Section B, passing the raw coverage
profile and the diff. The agent itself identifies which added lines the profile shows as
uncovered — no pre-computed list.

**Zero tolerance for coverage theater.** Tests must assert real behavior on the uncovered branch.
A line that genuinely can't be meaningfully tested (defensive/unreachable/generated code) gets
skipped with a stated reason — mirroring how `coderabbit-review` already skips stale findings
rather than forcing something — not padded with a tautological test. This mandate is absolute;
see CLAUDE.md's Testing Strategy / Test Quality sections for the underlying conventions
(table-driven tests, DI/mocks via `go.uber.org/mock/mockgen`, "test behavior not implementation").

After writing tests, re-run `atmos fix coverage` to confirm both green tests and improved coverage
on those exact lines — not just that new test files exist. If gaps remain that were judged
untestable, invoke the [`say` skill](../say/SKILL.md): `"PR <number> coverage check needs your
input."`

**Disclaimer, repeat it in any human-facing summary:** this check is scoped to the touched
packages' own tests, not full-suite (`-coverpkg=./...`) breadth — a fast approximation for
proactively suggesting tests on this patch, not a source of truth. CI's full-suite Codecov upload
remains authoritative.

## Git hygiene

Same discipline as the rest of the loop: verify the commit is signed
(`git log --show-signature -1`), `git add` only the files actually touched, plain (never force)
`git push`.

## Related

- **[`test-coverage-fix` agent](../../agents/test-coverage-fix.md)** — does the actual fixing,
  Section A (failing tests) then Section B (coverage gaps).
- **[`fix-all` skill](../fix-all/SKILL.md)** — invokes this skill at step 7, after the lint check
  (and reuses it directly at step 2 for CI-sourced Acceptance Tests failures). Scheduled hourly by
  [`pr-maintenance-loop`](../pr-maintenance-loop/SKILL.md), or run this directly for a one-shot.
- **[`say` skill](../say/SKILL.md)** — invoked on every human-attention exit path above.
- **[`lint` skill](../lint/SKILL.md)** — the sibling patch-scoped check for static analysis
  findings; kept separate since lint and test health are distinct concerns.
- `atmos fix coverage` / `atmos fix tests` (`.atmos.d/fix.yaml`) — the custom commands this skill
  runs; thin wrappers around `.claude/skills/test-coverage/scripts/patch-test-coverage.sh`.
