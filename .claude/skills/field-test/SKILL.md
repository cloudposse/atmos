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
repository's default branch (`gh repo view --json defaultBranchRef -q .defaultBranchRef.name`)
when no PR exists yet. Do NOT use the upstream tracking branch (`@{u}`) as the base — for a normal
feature branch that tracks `origin/<same-branch-name>`, diffing against its own upstream produces
an empty or near-empty diff, not the PR's actual changes, once the branch has been pushed.

A branch NAME from `gh` (e.g. `develop`) is not guaranteed to be a usable git ref in THIS checkout
— shallow clones, detached HEADs, and worktrees with a narrow fetch refspec can have the name
without the commits. Before running any diff, verify the base actually resolves here
(`git rev-parse --verify --quiet <candidate>^{commit}`), trying in order: `origin/<resolved-name>`,
`<resolved-name>` — take the first that verifies. **Do not fall back to `origin/main`/`main` when
`resolved-name` came from an actual PR base other than the default branch** (e.g. a PR targeting
`develop`) — diffing against `main` instead of the PR's real base compares against the wrong
history and can silently miss real changes or include unrelated ones. `origin/main`/`main` are
legitimate fallback candidates only in the no-PR case (`gh pr view` returned nothing, so
`resolved-name` already IS the repository's default branch) or when `gh` itself isn't available at
all. If a known, non-default PR base doesn't resolve as either `origin/<resolved-name>` or
`<resolved-name>`, stop and ask the user which base to diff against — do not run `git diff
<base>...` against an unverified ref and let it fail with a confusing git error, and do not
silently substitute a different branch for a known PR base. Once a real base is confirmed, inspect
the FULL set of
changes relative to that base — content, not just a file-list summary: `git diff <base>...HEAD`
(the full patch, not `--stat`, since `--stat` only shows file names and line counts, not what
those lines actually do) for committed history; `git diff HEAD` for any staged/unstaged changes
to tracked files not yet committed; and `git ls-files --others --exclude-standard` to enumerate
untracked files — `git status --porcelain` lists their paths too but never their content, and
neither `git diff HEAD` nor a `--stat` summary includes untracked files at all, so a brand-new
implementation file can otherwise go completely unread. Read the actual content of every
untracked file this turns up, the same as any diff hunk. Derive the test target from all of
that — the CLI command(s), flag(s), config option(s), or subsystem the changed files implement —
and state explicitly what you inferred and why before proceeding to Phase 1. Only fall back to
asking the user if no candidate base ref resolves at all, there's truly nothing changed (clean
worktree, base equals HEAD, no untracked files), or the changes span multiple unrelated features
with no coherent single target (ask which one to focus on, don't silently pick one).

Goal: catch "vibe-coded slop" — behavior that looks fine in code review but breaks or misleads a
real user — not to re-run what automated tests already cover. Anticipate plausible user
misunderstandings, not just obvious bugs.

**This pass is investigation only.** Do not fix anything you find — see Phase 5. Report and stop;
the user decides what to fix. Any follow-up fix work — including a plan to fix the findings, not
just the implementation — must close with the `fix-log` skill, not this one.

## Plan Mode

If plan mode is active when this skill is invoked (a `<system-reminder>` says so), Phase 3
(Build fixtures) and Phase 4 (Execute) can't run inline — writing fixture files and running real,
sometimes state-mutating commands are exactly what plan mode exists to gate.

- Run Phase 1 (Research) and Phase 2 (Generate hypotheses) as normal — both are already read-only
  and fit plan mode's constraints without modification.
- Instead of proceeding into Phase 3/4, write the plan file: the target under test, the
  prioritized hypothesis list from Phase 2, the fixtures Phase 3 would build (note which
  extend/copy an existing fixture, per that phase's guidance), and which planned Phase 4 commands
  are read-only vs. state-mutating — the mutating ones are specifically what need sign-off.
- Call `ExitPlanMode` to request approval of that plan. Don't ask for approval any other way.
- Once approved, resume at Phase 3 using the approved plan as the fixture/execution blueprint.

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
  defaulting from a branch diff, read the full diff content itself first (not just the post-change
  files, and not just a `--stat` summary) — the diff shows what changed *from*, which is where a
  regression or half-finished edge case would show up. Include untracked files
  (`git ls-files --others --exclude-standard`) in this reading pass too — they never appear in any
  diff at all.
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
- Any command that does real work across multiple items (batch installs/updates, concurrent
  workers, multi-resource loops) — does it show *live* progress, or does it silently buffer
  everything and dump it all at once when the whole batch finishes? A command that takes 10+
  seconds with zero output is indistinguishable from a hang to a real user. Also check whether its
  status lines actually use `ui.Success`/`ui.Error`/`ui.Warning`/`ui.Info` (icon + theme color) per
  CLAUDE.md's I/O and UI Usage section, rather than a hand-rolled glyph (`"✓ %s"`, `"✗ %s"`) printed
  through the plain `ui.Writef`/`ui.Write` — the two are easy to conflate since both compile and
  both "print a checkmark," but only the semantic function is themed/colored. **This class of bug
  is easy to miss when every command in this pass has been run through the Bash tool** — captured
  output can look fine in the transcript even when the real behavior (silent hang, unstyled text)
  would be obvious to a human watching a real terminal. This needs two *separate* checks, not one
  piped command doing double duty — piping through `cat -v` makes the command non-TTY, which
  exercises the non-live fallback renderer instead of the real live-progress path, and
  `--force-color` does not restore TTY behavior:
  - **Live progress**: run the command in a real pseudo-TTY, unpiped (e.g.
    `script -q /dev/null build/atmos toolchain update --force-tty --force-color`), and watch it —
    does a spinner/progress bar actually redraw in place, or does it silently buffer and dump
    everything at once?
  - **ANSI styling**: separately, pipe a `--force-color` run through `cat -v` (or grep for the raw
    `\x1b[` / `^[[` escape sequence) to confirm color codes are actually present around each status
    line, not just plain text with a Unicode glyph. Piping is fine here since this check only cares
    about styling, not live-rendering behavior.
- Any "N -> M" / diff-style report line — construct a case where N and M are the *same value in
  different string forms* (e.g. `v1.2.3` vs `1.2.3`, the one equivalence `normalizeVersion`
  actually handles by stripping a leading `v` — don't test casing or trailing-metadata variants
  unless the specific normalizer under test explicitly documents supporting them). A raw
  string-equality comparison will misreport a no-op as a change, which also tends to corrupt
  whatever summary/count line tallies outcomes — check the tally against the individual lines
  above it, don't just trust it.

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
- For any command with a summary/tally line (`Updated N, up to date M, failed K`), manually
  recount the individual lines above it and compare — don't trust the tally at face value. A
  miscounted summary is a strong signal the classification logic feeding it is wrong somewhere,
  not just a cosmetic issue.
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
to fix. If a plan gets made to fix any finding — whether right away or as a later follow-up,
even in a different session — that plan and its implementation must close with the `fix-log`
skill so the fix leaves a durable record under `docs/fixes/`. End by invoking the `say` skill — a
completed test pass reaching a stopping point a human should review is exactly its trigger.

## Related

- **Plan Mode** (above) — when invoked under plan mode, Phases 3-4 wait for approval via
  `ExitPlanMode` before building fixtures or executing.
- **`Explore` agent** — Phase 1's broad read-only research.
- **`atmos-emulator` skill** — real local infra for Phase 3/4 when the target touches
  AWS/GCP/Azure/Kubernetes.
- **`docs` skill** — conventions for the CLI docs being cross-checked in Phase 1.
- **`fix-log` skill** — required for any follow-up fix work, including a plan to fix findings,
  once the user decides what to fix; out of scope here.
- **`say` skill** — end-of-pass notification.
