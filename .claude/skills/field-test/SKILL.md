---
name: field-test
description: "Hands-on manual DX test pass of a feature or CLI command: read the real implementation and tests, hypothesize plausible user misunderstandings and misuse automated tests don't cover, build durable fixtures, execute for real against real state, and report ranked findings. Investigation only — never fixes anything found. Defaults to testing whatever the current branch changed vs its base branch when no explicit target is given. Invoke on explicit requests like 'field test X' / 'do a DX test pass on X' / 'find vibe-coded slop in X' / 'field test this branch'."
argument-hint: "Feature or command to test, e.g. 'atmos vendor pull' (omit to default to this branch's change)"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Field Test

Hands-on, adversarial test pass of **`$ARGUMENTS`** (the feature/command named when this skill
was invoked, e.g. `atmos vendor pull`).

**If no target was given, default to the change introduced on the current branch** rather than
asking. Resolve the actual pull-request base branch when one is available
(`gh pr view --json baseRefName -q .baseRefName` for the current branch), falling back to the
repository's default branch (`gh repo view --json defaultBranchRef -q .defaultBranchRef.name`, or
`origin/main`/`main` if `gh` isn't available) when no PR exists yet. Do NOT use the upstream
tracking branch (`@{u}`) as the base — for a normal feature branch that tracks
`origin/<same-branch-name>`, diffing against its own upstream produces an empty or near-empty
diff, not the PR's actual changes, once the branch has been pushed. Then inspect the FULL set of
changes relative to that base: `git diff <base>...HEAD --stat` for committed history, plus
`git status --porcelain` and `git diff HEAD` for any staged, unstaged, or untracked changes not
yet committed — a field test run before the day's work is committed must still see it. Derive the
test target from all of that — the CLI command(s), flag(s), config option(s), or subsystem the
changed files implement — and state explicitly what you inferred and why before proceeding to
Phase 1. Only fall back to asking the user if there's truly nothing changed (clean worktree, base
equals HEAD) or the changes span multiple unrelated features with no coherent single target (ask
which one to focus on, don't silently pick one).

Goal: catch "vibe-coded slop" — behavior that looks fine in code review but breaks or misleads a
real user — not to re-run what automated tests already cover. Anticipate plausible user
misunderstandings, not just obvious bugs.

**This pass is investigation only.** Do not fix anything you find — see Phase 5. Report and stop;
the user decides what to fix (and when they do, that follow-up work should close with the
`fix-log` skill, not this one).

## Phase 1 — Research before touching anything

The goal is a map of "documented or plausible usage" minus "already tested" = what needs manual
verification. This phase is broad, read-only research — delegate it to `Agent subagent_type:
"Explore"` (1-3 agents in parallel, one per bullet below) rather than doing it all serially inline.

When defaulting to the current branch (no explicit target given), scope every bullet below to the
target inferred from the branch diff — don't research the whole surrounding subsystem when the
branch only touched one corner of it. If the branch's changed files span more than one command or
package, treat each as a separate target to cover in Phase 2-4, prioritized by how much of the
diff each accounts for.

- **Implementation** — the actual code, not just its docs or the skill describing it. Inspect
  every changed production package identified by the diff — new business logic belongs in narrow
  `pkg/` packages per this repo's conventions, but `internal/exec/` is still where a large amount
  of existing logic lives during its ongoing migration, so a branch touching files there must
  still be read, not skipped. Check both `cmd/<command>/` (thin call site) and whichever
  `pkg/`/`internal/exec/` package(s) it delegates to for the real logic and error paths. When
  defaulting from a branch diff, read the diff itself first (not just the post-change files) —
  the diff shows what changed *from*, which is where a regression or half-finished edge case
  would show up.
- **Docs and skills** — every relevant page under `website/docs/cli/commands/`, the matching
  `.claude/skills/atmos-*` skill(s) for the subsystem, and any README describing the feature. Note
  anything phrased with confidence you haven't independently confirmed against the code — docs and
  skills describe intended behavior, not necessarily current behavior.
- **Existing automated tests** — unit tests colocated with the code, `tests/test-cases/` fixtures,
  `tests/testdata/` golden snapshots. For each, note exactly what it does and doesn't exercise
  (mocked vs. real execution, which flags/paths/backends are hit).
- **Every flag, config option, and documented action/mode** — grep for them and list them. You
  will need to touch every one in Phase 4.

## Phase 2 — Generate hypotheses, don't just wander

Before running anything, write down concrete things to try, prioritized by what a real user would
plausibly do:

- Every flag combination that seems natural but might not be validated (two flags that should be
  mutually exclusive; two config fields whose combination is never cross-checked).
- Every place the docs/skill claim something you haven't verified against actual code.
- Any "safe-looking" command (`plan`/`preview`/`--dry-run`/`list`/`describe`) that might secretly
  mutate state or trigger side effects, if built the same way as a mutating command — Atmos has
  many of these pairs (e.g. `terraform plan` vs `apply`, `vendor diff` vs `pull`), so this is a
  high-yield category here specifically.
- Any action/mode described in docs but not exercised by ANY test or example in the repo — those
  are the highest-yield targets; if nothing has ever run it for real, assume it's broken until you
  prove otherwise.
- Copy-paste/misconfiguration scenarios — what happens if a user copies a working stack/component
  block and changes one field but forgets a related one?
- Error messages — accurate, do they name the actual flags/values involved, do they suggest a fix
  (per this repo's error-builder/hint conventions)?
- Idempotency/rerun-safety — run the same operation twice; does the second run behave correctly?
- Determinism — run the same read-only command several times with no state change between runs —
  is the output identical every time?

## Phase 3 — Build real, durable fixtures

- Prefer extending or copying an existing fixture (`tests/test-cases/`, `examples/`, `demo/`) over
  inventing one from scratch.
- Build fixtures that exercise every documented capability, especially ones nothing in the repo
  currently exercises. Make them realistic, not minimal-to-the-point-of-artificial.
- If real infrastructure/emulators are available for what you're testing, use them for at least one
  pass — this repo ships local AWS/GCP/Azure/Kubernetes/Vault/registry emulators for exactly this
  purpose (see the `atmos-emulator` skill). Don't rely solely on mocked/dry-run paths, since that's
  exactly what's already covered by automated tests.
- Never manually edit golden snapshot files under `tests/test-cases/`, `tests/testdata/`, or
  `tests/snapshots/` — regenerate them via `-regenerate-snapshots` per CLAUDE.md's Golden Snapshots
  section.
- Keep fixtures that have lasting value (they close real coverage gaps); don't create
  scratch-and-delete throwaways unless truly one-off.

## Phase 4 — Execute for real, with discipline

- Run actual commands against a disposable fixture or emulator by default. Before any command that
  can mutate state, obtain explicit user confirmation. Run against shared or production state only
  with a documented backup and rollback plan. Don't reason abstractly about what "should" happen —
  observe what does happen. Build a fresh binary first (`atmos build`) if the change under test
  isn't already reflected in `./build/atmos`.
- Never pipe redirection into a command under test — per CLAUDE.md, piping breaks TTY detection,
  which can mask exactly the DX issues (interactive prompts, color, spinners) you're testing for.
- **Before every test, verify you're actually starting from a clean/expected state — don't
  assume.** Stale state from a previous run (yours or a prior session's) will silently corrupt your
  results. Reset explicitly and confirm the reset worked (check a resource count/id changed, not
  just that a command exited 0).
- When something surprises you, reduce it to the smallest reproducible case and verify the repro
  twice.
- If Phase 1 research made a claim, verify it live before trusting it — code-reading can miss
  control flow (e.g. assuming a flag is silently ignored when it actually errors, or vice versa).
  Correct the record explicitly when research turns out wrong.
- Test the happy path too, not just edge cases — confirm what's supposed to work actually does, so
  the report distinguishes real regressions from things that were never broken.

## Phase 5 — Report

For every finding: exact repro command(s), expected vs. actual output, and severity (silent data
loss/mutation > crash on reasonable input > confusing error message > cosmetic). Rank the report by
severity, most dangerous first. Explicitly call out anything verified as working correctly too — a
report that's only bad news is as misleading as one that's only good news.

Keep a running scratch log of findings as you go (in your own working notes/task list) rather than
reconstructing everything at the end from memory — but don't commit that log. Per CLAUDE.md's Git
section, scratch/research files never get committed; only the fixtures built in Phase 3 (if kept
for lasting value) and this final report are durable output.

Do not fix anything found — this pass is investigation only. Stop and report; the user decides what
to fix. End by invoking the `say` skill — a completed test pass reaching a stopping point a human
should review is exactly its trigger.

## Related

- **`Explore` agent** — Phase 1's broad read-only research.
- **`atmos-emulator` skill** — real local infra for Phase 3/4 when the target touches
  AWS/GCP/Azure/Kubernetes.
- **`docs` skill** — conventions for the CLI docs being cross-checked in Phase 1.
- **`fix-log` skill** — for the user's follow-up once they decide what to fix; out of scope here.
- **`say` skill** — end-of-pass notification.
