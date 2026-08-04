---
name: field-test
description: "Hands-on manual DX test pass of a feature or CLI command: read the real implementation and tests, hypothesize plausible user misunderstandings and misuse automated tests don't cover, build durable fixtures, execute for real against real state, and report ranked findings. Investigation only — never fixes anything found. Invoke on explicit requests like 'field test X' / 'do a DX test pass on X' / 'find vibe-coded slop in X'."
argument-hint: "Feature or command to test, e.g. 'atmos vendor pull'"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Field Test

Hands-on, adversarial test pass of **`$ARGUMENTS`** (the feature/command named when this skill
was invoked, e.g. `atmos vendor pull`). If no target was given, ask which feature/command to test
before starting.

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

- **Implementation** — the actual code, not just its docs or the skill describing it. Per this
  repo's conventions, business logic lives in narrow `pkg/` packages, not `internal/exec/` (being
  phased out) — check both `cmd/<command>/` (thin call site) and the `pkg/` package(s) it
  delegates to for the real logic and error paths.
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
