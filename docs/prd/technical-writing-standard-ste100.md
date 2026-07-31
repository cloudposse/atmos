# Technical Writing Standard: ASD-STE100 for PRDs and Docs

## Overview

This PRD adopts a writing style for Atmos PRDs (`docs/prd/`) and website
documentation (`website/docs/`). The style draws on **ASD-STE100**
(Simplified Technical English) principles. This PRD also adds the tooling
that checks the style: a `vale` configuration, a new `atmos lint docs`
custom command, a pre-commit hook, and a CI workflow. The rollout is
non-blocking today and grows stricter over three phases.

This document itself follows the style it proposes.

## Problem Statement

### Current State

Atmos has around 218 PRDs in `docs/prd/` and around 815 pages in
`website/docs/`. No shared writing style governs them. Sentence length,
voice, and word choice vary by author. Two automated checks touch prose
today, and neither covers style: `godot` checks that Go source comments end
with a period, and `lychee` checks that markdown links resolve. Nothing
checks sentence length, passive voice, contractions, or word choice.

### Challenges

- Long, multi-clause sentences slow readers down and hide the main point.
- Passive voice hides the actor: a reader cannot always tell what does an
  action.
- Contractions and wordy phrases add noise without adding meaning.
- New contributors have no reference for the expected style, so each PRD
  reads differently.
- The existing corpus exceeds 1,000 files. A full rewrite is not practical
  and would block unrelated work for months.

## Solution

Adopt a writing style inspired by ASD-STE100, and enforce it with `vale`,
wired through an Atmos custom command.

### Why ASD-STE100

ASD-STE100 (Simplified Technical English) is a controlled-language standard
the aerospace and defense industry created for maintenance manuals. It
exists to remove ambiguity: one sentence carries one meaning, so a technician
under time pressure, or a reader who does not speak English as a first
language, cannot misread it. The same properties suit software
documentation: PRDs and reference docs benefit from short sentences, active
voice, and consistent terminology.

### Copyright Constraint and Mitigation

ASD (Brussels) owns the copyright of the ASD-STE100 specification and its
controlled dictionary of approximately 900 approved words. The
specification states: "no reproduction or publication of it, in whole or in
part, shall be made without the written authority of an officer of ASD."
Atmos does not hold this authority.

This PRD therefore does not copy the specification text or the dictionary.
Instead, it defines the **Atmos writing style**: our own paraphrase of
ASD-STE100's principles (short sentences, active voice, no contractions,
plain words), plus a small technical dictionary of Atmos-specific terms
(`stack`, `component`, `Terraform`, and similar words), built with Vale's
own Vocab mechanism. This mirrors the approach every unofficial open-source
ASD-STE100 tool takes today (for example, `nuelcyoung/asd-ste100`): explain
the principles in original words, and link to the authoritative source for
readers who want the full specification.

`docs/writing-style-guide.md` holds the full rule list and the technical
dictionary. `https://www.asd-ste100.org/` holds the official specification.

### Architecture: Atmos-Native Tooling

A single command, `atmos lint docs`, checks the style. The Atmos toolchain
installs `vale` the first time the command runs — no manual install step,
and no separate setup action in CI. The same command runs in three places:

```text
Author writes or edits a PRD or a docs page
              │
              ▼
    atmos lint docs [--changed]
              │
    ┌──────────┼───────────────────┐
    │          │                   │
locally   pre-commit hook     CI: .github/workflows/docs-lint.yml
          (vale-docs-lint)    (runs inside the ghcr.io/cloudposse/atmos
                                container image)
              │
              ▼
    vale --config=.vale.ini <files>
              │
    ┌──────────┴───────────────────────────────┐
    │                                           │
.vale/styles/Atmos/*.yml            .vale/styles/config/vocabularies/Atmos/
(the four paraphrased rules)         {accept,reject}.txt (technical dictionary)
```

### Key Design Principles

1. **One command everywhere.** `atmos lint docs [--changed]` runs the same way locally, in the pre-commit hook, and in CI. No separate code path can drift from what a contributor sees on their own machine.
2. **Toolchain-managed installation.** `vale` is a `dependencies.tools` entry on the command, resolved through the `vale-cli/vale` Aqua registry alias. Atmos installs it automatically wherever the command runs.
3. **No copyrighted content.** The rule set and dictionary are original work, inspired by ASD-STE100 but not a copy of it.
4. **Non-blocking rollout.** Phase 1 sets every rule to `suggestion` level. A suggestion never fails a commit, a pre-commit run, or a CI check.
5. **Patch-scoped enforcement.** The `--changed` flag lints only files that differ from `origin/main`, the same pattern `atmos lint --changed` already uses for `golangci-lint`.

## Implementation

### 1. Vale Configuration (`.vale.ini`)

Sets `StylesPath = .vale/styles`, scopes the `Atmos` style to `docs/prd/*.md`
and `website/docs/**/*.{md,mdx}`, maps the `.mdx` format to `.md` for
parsing, and sets `MinAlertLevel = suggestion`.

### 2. Custom Rules (`.vale/styles/Atmos/*.yml`)

Four hand-authored rules approximate ASD-STE100 principles:

| Rule | Checks | Approximates |
|---|---|---|
| `SentenceLength` | Sentences over roughly 25 words | ASD-STE100's short-sentence rule |
| `PassiveVoice` | Likely passive constructions | ASD-STE100's active-voice rule |
| `Contractions` | Contractions such as "don't" | ASD-STE100's no-contractions rule |
| `WordyPhrases` | Wordy phrases such as "in order to" | ASD-STE100's plain-word rule |

### 3. Technical Dictionary

`.vale/styles/config/vocabularies/Atmos/accept.txt` lists Atmos and
infrastructure terms that stay allowed (`Terraform`, `component`, `stack`,
and similar words). `reject.txt` lists a short, generic list of words to
avoid, unrelated to ASD-STE100's own dictionary.

### 4. Atmos Custom Command

`.atmos.d/lint.yaml` gains a `docs` subcommand, next to the existing
`link-check` subcommand:

```yaml
- name: docs
  description: Lint technical docs prose (Atmos writing style, via vale)
  flags:
    - name: changed
      type: bool
      default: false
  dependencies:
    tools:
      vale: "3.16.0"
  steps:
    - type: shell
      command: |
        # scoped by `git diff` when --changed is set, else the full docs tree
        vale --config=.vale.ini $files
```

`.atmos.d/toolchain.yaml` gains a `vale: vale-cli/vale` alias so the command
can refer to the tool by its short name.

The command stays a separate subcommand rather than joining the shared
`lint changed` pipeline. This keeps `atmos lint --changed` and `make lint`
unchanged for every existing caller while the rule set is still
informational. Phase 2 revisits this choice.

### 5. Pre-Commit Hook

`.pre-commit-config.yaml` gains a local hook, `vale-docs-lint`, that runs
`atmos lint docs --changed` on every commit. It mirrors the existing
`atmos-validate-editorconfig` hook's shape: a system-language entry, no
filename passing, always runs.

### 6. CI Workflow

`.github/workflows/docs-lint.yml` runs the same `atmos lint docs --changed`
command inside the `ghcr.io/cloudposse/atmos` container image, on pull
requests that touch `docs/prd/`, `website/docs/`, or the Vale
configuration. No third-party Vale or reviewdog action runs; the container
already has `atmos`, and the toolchain installs `vale`.

### 7. Skill and Documentation Updates

- A new skill, `.claude/skills/writing-style/SKILL.md`, gives an
  agent-facing summary of the rules and points to `docs/writing-style-guide.md`
  for detail.
- `.claude/skills/docs/SKILL.md` gains a "Writing Style" section and an
  `atmos lint docs --changed` step in its validation checklist.
- `.claude/skills/changelog/SKILL.md` gains one line that draws the
  boundary: blog posts follow that skill's narrative rules, not this
  standard.
- `CLAUDE.md` gains a command reference and a cross-reference from "PRD
  Documentation (MANDATORY)" to the `writing-style` skill.

## Testing Strategy

- `atmos lint docs` against the full existing corpus (1,033 files today)
  completes with zero errors and zero crashes, confirming the configuration
  loads and every rule runs without breaking the scan.
- `atmos lint docs --changed` on a branch with new and modified files
  confirms the `git diff` scoping picks up exactly those files, and reports
  "No changed docs to lint" when nothing in scope changed.
- `pre-commit run vale-docs-lint` confirms the hook executes and reports
  Passed under the default `suggestion` severity.
- Direct `vale --config=.vale.ini` runs against sample pages confirm each of
  the four rules fires on a matching pattern and stays silent otherwise.
- `.github/workflows/docs-lint.yml` needs a push to a pull request branch
  for full validation; a GitHub Actions run cannot execute locally. This PR
  documents that limitation rather than skipping it silently.

## Migration Path

### Phase 1 — Ship (this PR)

The tooling lands with `MinAlertLevel = suggestion`. Every finding is
informational. Nothing blocks: not a local run, not the pre-commit hook, not
CI.

### Phase 2 — Promote (future PR)

After a validation window against real pull requests, promote the rules
with the lowest false-positive rate to `warning` or `error`, scoped to
changed lines only. Consider folding `lint docs --changed` into the shared
`lint changed` pipeline once the rule set proves stable.

### Phase 3 — Expand (future PR)

Add further checks (for example, noun-cluster length, tense consistency, or
procedure-step structure). Existing files stay grandfathered until an
author edits them; this PRD does not propose a retroactive rewrite.

## Risks & Mitigation

| Risk | Mitigation |
|---|---|
| Reproducing ASD-STE100's copyrighted specification or dictionary | The rule set and dictionary are original, paraphrased work; `docs/writing-style-guide.md` links to the official specification instead of copying it. |
| Regex-based heuristics (passive voice, sentence length) produce false positives and erode trust | Phase 1 keeps every rule at `suggestion` level; Phase 2 promotes severity only after measuring the false-positive rate on real PRs. |
| Over 1,000 existing files do not comply | The `--changed` flag scopes every check to the current patch; existing files stay grandfathered until edited, not rewritten. |
| A contributor's environment cannot reach the Aqua registry to install `vale` | The Atmos toolchain caches the binary after the first successful install; the command still runs against zero changed files without failing. |
| Contributors see the rules as arbitrary | `docs/writing-style-guide.md` states the reason behind every rule, tied to the ASD-STE100 principle it approximates. |

## Success Criteria

- [ ] `atmos lint docs` completes with zero errors and zero crashes across
      the existing docs corpus.
- [ ] `atmos lint docs --changed` completes in well under a minute on a
      typical pull request diff.
- [ ] The pre-commit hook, the CI workflow, and a local run all invoke the
      identical `atmos lint docs` command.
- [ ] `docs/writing-style-guide.md` documents every active rule and the
      Atmos technical dictionary.
- [ ] No text from the official ASD-STE100 specification or its controlled
      dictionary appears anywhere in the repository.

## Alternatives Considered

**Full retroactive rewrite of all existing docs.** Rejected. The cost is
high, the risk of introducing factual errors during a mechanical rewrite is
high, and it would block unrelated documentation work for an extended
period.

**Reproduce the ASD-STE100 dictionary directly, as a Vale spelling style.**
Rejected. This would violate ASD's copyright.

**Use a third-party `vale-action` plus `reviewdog` in CI.** Rejected. This
adds a dependency outside Atmos's own toolchain and creates a second code
path that could drift from the command contributors run locally. The
Atmos-native command, run inside the existing container image, gives the
same patch-scoped result with less surface area to maintain.

**Adopt only an existing generic Vale style, such as "Google" or
"write-good," instead of a custom rule set.** Rejected as the sole option.
These styles are useful, but none targets ASD-STE100's specific
combination of rules (no contractions, this project's chosen wordy-phrase
list, an Atmos-specific technical dictionary). Nothing prevents adding one
of these styles as an additional `BasedOnStyles` entry later.

**Enforce blocking severity immediately.** Rejected. No prose linting exists
today; a sudden blocking gate would fail most open pull requests and create
avoidable churn. The phased Migration Path above exists for this reason.

## References

- [ASD-STE100 home page](https://www.asd-ste100.org/) — the authoritative
  specification.
- [Vale documentation](https://vale.sh/docs/) — the linter this PRD adopts.
- `docs/writing-style-guide.md` — the full rule list and technical
  dictionary.
- `.claude/skills/writing-style/SKILL.md` — the agent-facing skill.
- `.atmos.d/lint.yaml` — the `atmos lint docs` custom command.

## Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0.0 | 2026-07-31 | Initial adoption: Vale configuration, custom rules, technical dictionary, `atmos lint docs` command, pre-commit hook, CI workflow, `writing-style` skill, and style guide. |
