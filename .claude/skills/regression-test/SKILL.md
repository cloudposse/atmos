---
name: regression-test
description: "Compare Atmos revisions for upgrade regressions: behavior changes, broken existing workflows, new side effects, and slowdowns. Use for 'regression test', 'compare main against the previous release', or 'check upgrade compatibility'. Assess existing and potential config edition protection. Investigation only; general feature/DX exploration belongs to field-test, and isolated slowdown investigations to performance-regression."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Regression Test

Answer: **What happens differently to an existing user who upgrades without changing their project?**

Investigate and report. Do not fix product code, implement edition transitions, regenerate
goldens, publish findings, or create issues as part of this skill. The number of bugs found
is not a success metric. Unchanged bugs do not count as new regressions, but a worsening
of an existing bug can. New functionality matters here when it affects existing workflows.

Use the hypothesis-driven, real-execution discipline of [field-test](../field-test/SKILL.md),
with the comparative evidence and edition analysis below. Passing today's unit tests alone
does not establish upgrade compatibility. Under plan mode, research and specify the experiments;
wait for execution mode before writing fixtures or executing the investigation.

## 1. Freeze the comparison

Interpret supplied baseline, candidate, and subsystem arguments. Otherwise:

- Resolve the latest **published, non-draft, non-prerelease** release through GitHub release
  metadata (`gh api repos/cloudposse/atmos/releases/latest`). A newer RC tag is not the default.
- Resolve remote `main`, fetching missing/stale refs when necessary. Pin both endpoints to
  commit SHAs; record the release tag, publication time, resolution time, and candidate SHA.
  Never substitute the current feature branch or include its uncommitted code in the candidate.
- Verify refs and ancestry. Use the endpoint diff (`git diff <baseline-sha> <candidate-sha>`),
  not a merge-base diff that silently changes the baseline. If refs cannot be obtained, report
  the unavailable comparison rather than guessing a different release.
- Preserve the user's checkout and changes. Build isolated sources under a unique
  `.context/regression-test/<run>/` directory; record source, toolchain, build settings, binary
  hashes, and harness revision. Distinguish locally rebuilt releases from published binaries.
  If both endpoints are identical, report that there is no revision delta to investigate.

## 2. Generate upgrade hypotheses

Read the production diff content, implementation on both sides, relevant existing tests,
and migration/release documentation. Map changed shared code to its **existing callers**;
an unchanged command can regress when its loader, runner, cache, or output writer changes.

Read [historical-patterns.md](references/historical-patterns.md) and search `docs/fixes` for
the changed subsystems. Verify historical claims against code and live execution. Fix logs
provide hypotheses and realistic fixture shapes, not proof of a current regression.

Create a prioritized coverage ledger: hypothesis, user impact, command/configuration,
fixture source, expected comparison, and evidence/status. Cover changed behavior first,
plus historical sentinel cases for configuration/merge/path semantics and machine output.
Account explicitly for relevant flag/env/config precedence, lazy evaluation, side effects,
credential and cache isolation, concurrency, cancellation, and performance. Mark untested
areas rather than claiming whole-product coverage.

Do not exclude a useful differential experiment merely because unit tests cover that path.
Conversely, a new command unsupported by the baseline is not itself a regression.

## 3. Analyze editions, including gates that could be introduced

Read [editions](../editions/SKILL.md), `docs/prd/editions.md`, the journal in `pkg/edition`,
and the changed code's actual edition/config consumers. For each meaningful difference:

1. Establish whether intent is documented or unresolved. Do not infer intent from a changed
    test expectation or a new default alone.
2. Test any existing protection end to end. Compare the **same configuration on both
    binaries** with a pin before the relevant transition, the boundary date, no pin, and
    relevant explicit overrides. Use the actual journal date, not an assumed release date.
    Where pin parsing or precedence changes, also exercise partial dates, invalid pins, and
    flag > environment > config precedence. Keep invalid-input failures separate from gates.
3. If protection is missing or ineffective, assess whether introducing a transition can
    preserve release behavior. Use **KindValue** for default changes; use **KindBehavior**
    when a stored value keeps its spelling but changes meaning or execution. New keys that
    replace previously unconditional behavior also need this assessment.
4. Recommend the transition's old/new behavior, affected settings/call sites, retained
    compatibility path, and tests proving old pins preserve behavior and newer/unpinned
    configurations select the intended change. Do not implement the recommendation.

A journal entry is not a compatibility path. Trace `BehaviorChanged` to its callers and
verify execution; historical candidate notes may remain ungated. Explicit configuration
must retain its documented precedence over edition-derived defaults. Never change a user's
pin solely to make a failing comparison pass: label such an experiment as mitigation.

Classify an intentional, correctly gated transition as a compatibility finding, with its
unpinned upgrade impact. Report missing/ineffective protection explicitly. Accidental crashes,
wrong values, isolation failures, and slowdowns remain regressions even if a gate is proposed.
Recommend correcting pure regressions for every edition. Edition transitions preserve
intentional behavior changes; do not recommend gating a fix to preserve a defect.
Do not recommend preserving unsafe behavior merely to obtain equality; identify cases that
need a direct fix or an explicit migration instead.

## 4. Run paired experiments

Prefer copies of release-compatible fixtures from `tests/fixtures/scenarios`, examples, or
historical reproducers. Use identical fixture content for both binaries, including the same
external tool versions. Preserve fixture origins and any modifications. A current fixture
using new syntax cannot establish that an old workflow regressed.

- Use equivalent initial state, isolated homes/config/cache directories, and controlled
  dependencies. Strip ambient Atmos/cloud credentials and settings; use disposable local
  fixtures or emulators for mutation. Verify the reset before each run. Do not touch live
  infrastructure or external reporting endpoints without task-specific authorization.
  Give fixtures nested in this checkout their own disposable Git root: Atmos discovers
  `.atmos.d` at the Git root even when `ATMOS_CLI_CONFIG_PATH` is explicit. Inspect resolved
  config to ensure repository development commands/settings did not leak into the experiment.
- Capture exact commands, environment overrides, stdout, stderr, exit status, resolved data,
  and relevant filesystem/backend/subprocess effects. Preserve raw observations. Normalize
  only identified incidental differences (e.g. sandbox roots); never normalize away missing
  values, type changes, ordering guarantees, channel pollution, or errors.
- Run structured-output tests as real captured subprocesses and parse their stdout. Test
  interactive behavior separately in a PTY; piping does not preserve TTY behavior.
- Reproduce unexpected differences twice from clean state and reduce confirmed findings to
  a small reusable fixture. For intermittent failures, retain trials and failure counts;
  failure to reproduce is inconclusive, not proof of compatibility.
- Verify actual downstream effects: parsed flags alone do not prove propagation, exit zero
  does not prove valid data, and a dry-run flag does not prove absence of writes or downloads.
- For cache/concurrency changes, verify values and isolation as well as work counts; exercise
  cold/warm state and different callers/identities where relevant. For cancellation, observe
  child-process termination, not just parent exit.

For timing use [performance-regression](../performance-regression/SKILL.md) and the maintained
`magefiles/README.md` contract. Rebuild both SHAs through `go tool mage bench:fieldTest`, with
the same current CLI harness, at least ten samples, and no concurrent builds or heavy tests.
Use `ATMOS_BENCH_FIELD_FILTER` to select relevant workloads. Keep correctness checks outside
timing; an older binary failing a new assertion may be a pre-existing bug, not a new slowdown.
Retain that failure before selecting another timing workload. Do not relax assertions/budgets
to make the comparison pass. Report medians, absolute/percentage differences, samples,
significance, and operation counts using the configured threshold (normally >=10%, p<0.05).
Do not compare local Mac measurements with Linux CI history or call harness allocations CLI
memory. Environmental degradation and unsupported workloads are not confirmed regressions.

## 5. Report and stop

Keep evidence and the report under the run directory. Each finding needs versions, fixture,
command, relevant edition/config, baseline and candidate observations, reproduction evidence,
user impact/severity, and edition assessment. Link the exact artifacts. Distinguish observed
behavior from inferred causes; identify a responsible commit only when verified.

Separate:

- **Confirmed regressions**, ranked by impact (data/credential corruption, incorrect operations,
  crashes/broken workflows, slowdown, then presentation changes).
- **Intentional compatibility changes**, including demonstrated protection, missing protection,
  and recommendations for transitions that could be introduced.
- **Preserved behavior and improvements**, with the workloads actually exercised.
- **Inconclusive cases and coverage gaps**, including unavailable dependencies/platforms,
  baseline failures, unchanged bugs relevant to the experiment, and untested hypotheses.

Do not promote unknown-intent differences to confirmed accidental regressions without evidence;
retain them as unresolved compatibility findings. State “no regressions observed in these
workloads” when warranted, never “regression-free.” A proposed edition gate is not a resolution.
Follow-up fixes are a separate task and use the repository's `fix-log` workflow.
