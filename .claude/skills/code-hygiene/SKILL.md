---
name: code-hygiene
description: "Reviews code changes for architectural smells that mechanical checks (lint, tests, coverage) structurally can't catch: duplicated 'shared' abstractions, missing sentinel errors, generics that discard their own type info, self-aware nolint suppressions of mandated rules, business logic in the wrong layer, admitted-but-unshipped gaps, config/flags that validate but silently no-op or always error, features that are implemented but never wired up, and documentation/code mismatches in either direction (docs describing an unimplemented or since-changed feature, or a shipped feature with no docs at all). Distinct from the general code-review skill (bugs/security/perf) and from lint (mechanical/syntactic) — this catches CLAUDE.md architectural-mandate violations that need reading intent, not just syntax. Patch-scoped against origin/main by default; a full-repo sweep runs only on an explicit human request. Invoke on explicit requests like \"check for vibe coding\" / \"run code hygiene\" / \"audit this PR's architecture\", or automatically as a step in the fix-all cycle."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Code Hygiene

A narrow, calibrated review pass — not a general bug hunt. `lint` catches syntactic violations,
`test-coverage` catches untested lines, and a normal correctness-focused review catches bugs — none
of those structurally catch an abstraction that got duplicated instead of reused, a feature that
validates but silently does nothing, or a comment that admits a gap nobody closed. This skill's job
is specifically the class of smell that only shows up when you read code against this repo's own
architectural mandates (CLAUDE.md) and ask "does this actually do what it claims to," not just
"does this compile and pass its own tests."

**This skill reports; it does not redesign.** A real architectural smell usually needs a design
decision, not a mechanical patch — report it clearly enough that a human (or a follow-up planning
pass) can make that call. See "Auto-fix policy" below for the one narrow exception.

## Scope: patch-aware by default

Default to the same scoping convention as `lint`/`test-coverage`: review only the files/packages
touched by `git diff origin/main...HEAD --name-only` (or the currently open PR's diff). This never
goes hunting through the other 340+ packages the current patch didn't touch. Unlike `lint` (which
wraps a real `atmos fix lint` command with an actual `--new-from-rev` flag), this skill has no
underlying CLI command — there's no literal flag to pass, only the phrasing below.

## Full-repo mode (explicit only)

Only when a human explicitly asks for a full sweep (e.g. "audit the whole repo", "full code
hygiene sweep" — never inferred, never run from `fix-all`'s automated cycle). A whole-repo pass is
expensive and belongs in an on-demand invocation, not an hourly loop.

## Dedup: skip a re-run against an unchanged diff

Before reviewing, hash `git diff origin/main...HEAD` (the diff content itself — this skill's
dedup key is "has the patch changed," not an external event set) and compare against
`.claude/state/code-hygiene/<branch-slug>.json`'s last-recorded hash. The `.claude/state/<name>/
<branch-slug>.json` file shape is borrowed from `.claude/hooks/security-remediate-trigger.sh` — the
one existing precedent for this pattern in this repo, not an established multi-skill convention —
adapted here to hash the diff instead of a set of alert IDs, since what's being deduped against is
different (a review cycle re-running on unchanged code, not a repeated external notification). If
the hash matches, skip re-reviewing and reuse the cached `findings` array
from that state file — report it as a one-line no-op citing the cached result count, don't burn
tokens re-deriving the same answer from an unchanged patch. If the hash differs (or no state file
exists yet), do the full review below, then write `{hash, findings, ts}` back to the state file
after reporting.

## The checklist

For each touched file/package, check for:

1. **Duplicated "shared" abstraction.** Near-identical logic (same loop/branch structure, same
   algorithm) appears two or more times inside a package whose entire purpose is to be *the*
   shared implementation — e.g. two separate tree-walk-and-hash functions in a package that only
   needs one, parameterized by what varies between the call sites.

2. **Missing sentinel errors.** A new or heavily-touched package returns errors but has zero
   `errors.New`/sentinel `var` declarations backing them — every error site is an ad-hoc
   `fmt.Errorf("... %w", err)` string literal instead of a wrapped static sentinel, violating
   CLAUDE.md's "All errors MUST be wrapped using static errors" mandate.

3. **A generic that discards its own type.** `func f[T A | B](...)` whose body immediately does a
   runtime type-switch/type-assert on `T` (`switch any(x).(type) { case A: ...; case B: ... }`) on
   every call path — the generic buys nothing over just accepting `any` or, better, unifying `A`
   and `B` into one real type. This is a strong "these two types should be one type" signal, not a
   legitimate use of generics.

4. **Self-aware suppression of a mandated rule.** A `//nolint`, `//revive:disable`, `#nosec`, or
   similar suppression comment that turns off a rule CLAUDE.md explicitly mandates (Options
   Pattern, cyclomatic complexity, file-length limits, sentinel errors) — especially one whose own
   comment text admits the violation ("too many params", "TODO: refactor", "temporary").

5. **Business logic in the wrong layer.** A `cmd/*.go` file containing loops, external API/network
   calls (git, GitHub, HTTP), or non-trivial branching beyond flag parsing and dispatch — that
   logic belongs in `pkg/`. Also flag any *new* file added under `internal/exec/` — this repo has a
   standing direction to stop growing that package; new abstractions belong under `pkg/`.

6. **Admitted-but-unshipped gap.** A `TODO`/`FIXME`/"not yet"/"doesn't support X yet" comment on a
   code path reachable from a documented, non-experimental command or config field, where the gap
   itself is *not* reflected anywhere a user would see it (`--help`, Docusaurus docs, error
   message) as "not supported."

7. **Fake/stub feature — validates but doesn't work.** A config field or CLI flag that passes
   schema/flag validation but whose only runtime behavior is a hardcoded no-op or an unconditional
   error — the schema is lying about what's usable today.

8. **Implemented but never wired up.** An exported function with real test coverage but zero
   non-test callers anywhere in the repo, especially one whose name implies it backs a user-facing
   command that doesn't actually expose it.

9. **Documentation/code mismatch, either direction.** Check every command, flag, or config field
   this patch adds, changes, or removes against the docs that describe it (Docusaurus pages under
   `website/docs/`, `docs/prd/*.md`, agent-skill reference docs, `--help` text) for all four
   failure modes:
   - **Docs describe a feature that isn't actually implemented** — aspirational/promised
     documentation for something that doesn't exist yet, or no longer exists, in the code.
   - **Docs describe how something used to work** — accurate once, but a later change in this
     patch (or a recent one) shifted the real behavior and nobody updated the prose describing it.
   - **Missing documentation** — a new, shipped, non-experimental command/flag/config field with
     no corresponding doc update at all, per CLAUDE.md's own "All new commands/flags/parameters
     MUST have Docusaurus documentation" and "Update all schemas... when adding config options"
     mandates.
   - **Implemented but undocumented** — the same failure as above, viewed from the code side: a
     capability that's fully built and reachable, but a user reading the docs would never learn it
     exists.

## What NOT to flag (avoid false positives)

- A stub that's honestly documented as unavailable (`--help` text, Docusaurus docs, or an
  intentionally loud error naming exactly what's missing and why) — the smell is a *silent* or
  *misleadingly-validated* stub, not a disclosed one.
- Legitimate generics whose body does real compile-time-dispatched work, not a runtime type-switch
  on every path.
- Pre-existing code outside the current patch, in default (patch-aware) mode — that's what an
  explicit full-repo sweep is for.
- Errors already using this repo's sentinel pattern correctly (`errors/errors.go` or a
  package-local `errors.go` mirroring it).
- `TODO`/stub markers inside test files, example fixtures, or explicitly experimental
  (`ATMOS_EXPERIMENTAL`-gated) commands — those are expected to be incomplete by design.
- A doc gap on code outside the current patch, in default (patch-aware) mode — flag it only if
  this patch touched the feature or its docs; a pre-existing, unrelated doc gap belongs to an
  explicit full-repo sweep, not a routine cycle.
- Internal/unexported helpers, and code under `internal/`, that were never meant to have
  user-facing docs in the first place — item 9 is about user-facing surface area, not every
  function needing a comment.

## Reporting

Use the `ReportFindings` tool, most-severe first (empty array if nothing survives). For each
finding, cite the exact file/line and which checklist item (1-9 above) it matches, plus a concrete
failure scenario (what a user configures/calls, and what silently goes wrong). Mark `CONFIRMED`
when the code unambiguously matches a checklist item, `PLAUSIBLE` when it's a suspicious pattern
that needs a human's architectural judgment call either way — don't force a PLAUSIBLE into
CONFIRMED just to simplify the report.

## Auto-fix policy

Only one class of finding is safe to fix without a human decision: **item 2 (missing sentinel
errors)**, and only when the fix is a pure mechanical conversion — replace a dynamic
`fmt.Errorf`/`errors.New` string with a declared sentinel + `%w` wrap, preserving the exact
original message text and error chain. Nothing else on the checklist gets auto-fixed. Items 1, 3,
5, 6, 7, and 8 are architectural judgment calls by nature — report them, don't guess at a fix.
Item 9 (doc/code mismatch) is never auto-fixed either, even though it can look mechanical: writing
accurate docs means following this repo's Docusaurus template and house style, not just filling a
gap with the first plausible sentence — report it and let a human (or a dedicated doc pass) write
the actual content. Item 4 (nolint suppression) is judgment-adjacent: report it, and only remove
the suppression yourself if you also fix the underlying violation in the same pass; never just
delete a `//nolint` comment and leave the violation live.

## Related

- **[`fix-all` skill](../fix-all/SKILL.md)** — invokes this skill at its own step 8 as part of the
  hourly/on-demand PR-readiness cycle; an unresolved finding blocks the "ready for final review"
  state the same way an unfixed lint finding or an invalid-but-unresolved CodeRabbit thread does.
- **[`lint` skill](../lint/SKILL.md)** — catches mechanical/syntactic violations `golangci-lint`
  can express as a rule; this skill catches the semantic/architectural ones that require reading
  intent, which a linter structurally cannot do.
- **`code-review`** (native skill) — general bug/security/performance/simplification review, not
  scoped to this repo's own CLAUDE.md architectural mandates. Use `code-review ultra` instead of
  this skill when you want a deep, adversarially-verified, multi-agent pass across a whole PR.
- `.claude/hooks/security-remediate-trigger.sh` — the precedent this skill's dedup-by-hash pattern
  is modeled on, applied inline here instead of as a separate hook.
