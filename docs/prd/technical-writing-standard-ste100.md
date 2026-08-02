# Technical Writing Standard: ASD-STE100 for PRDs and Docs

## Overview

Atmos PRDs and documentation pages have no shared writing style, and nothing checks one. This PRD adopts a style inspired by ASD-STE100 (Simplified Technical English). It also builds the tooling to check that style: a `vale` configuration, a new `atmos lint docs` custom command, a pre-commit hook, and a CI workflow. The rollout starts non-blocking and tightens over three phases, described below.

This document is written in the style it proposes. It does not clear every linter finding, and that is the intended outcome rather than an oversight: several of its sentences run past the 30-word threshold while carrying one connected idea. See the Migration Path and the style guide's "A `vale` finding is a prompt, not a mandate" section.

## Problem Statement

### Current State

Atmos has around 218 PRDs in `docs/prd/` and around 815 pages in `website/docs/`, written by many different authors over several years, so sentence length, voice, and word choice vary widely from one document to the next. Two automated checks touch prose today — `godot`, which requires Go source comments to end with a period, and `lychee`, which checks that markdown links resolve — but neither looks at style. Nothing today checks sentence length, passive voice, contractions, or word choice.

### Challenges

- Long sentences with several clauses chained together slow readers down and bury the point.
- Passive voice hides the actor, so a reader often cannot tell who or what performed an action.
- Contractions and wordy phrases add noise without adding meaning.
- New contributors have no reference for the style we expect, so every PRD reads a little differently.
- The existing corpus is too large — over a thousand files — to rewrite by hand without stalling unrelated work for months.

## Solution

Adopt a style inspired by ASD-STE100, and enforce it with `vale` through an Atmos custom command.

### Why ASD-STE100

ASD-STE100 (Simplified Technical English) is a controlled-language standard that the aerospace and defense industry built for maintenance manuals, because ambiguity in a maintenance instruction can be dangerous. Its central idea — one sentence, one meaning — travels well beyond aircraft manuals: a reader under time pressure, or a reader who does not speak English as a first language, benefits from the same clarity in a PRD or a reference doc. Short sentences, active voice, and consistent terminology serve software documentation for the same reason they serve a maintenance technician.

### Copyright Constraint and Mitigation

ASD (Brussels) owns the copyright to the ASD-STE100 specification and its controlled dictionary of roughly 900 approved words. The specification reserves all reproduction and publication rights to ASD, so nobody may copy or publish any part of it without written authorization from an ASD officer. Atmos holds no such authorization.

So this PRD does not copy the specification text or the dictionary. Instead, it defines the Atmos writing style: our own paraphrase of ASD-STE100's principles — short sentences, active voice, no contractions, plain words. The style also includes a small technical dictionary of Atmos-specific terms (`stack`, `component`, `Terraform`, and similar words), built with Vale's own Vocab mechanism. Every unofficial open-source ASD-STE100 tool takes the same approach today (for example, `nuelcyoung/asd-ste100`): explain the principles in your own words, and link to the authoritative source for anyone who wants the full specification.

`docs/writing-style-guide.md` holds the full rule list and the technical dictionary; `https://www.asd-ste100.org/` holds the official specification.

### Architecture: Atmos-Native Tooling

One command, `atmos lint docs`, checks the style. The Atmos toolchain installs `vale` the first time that command runs, so there is no manual install step and no separate setup action in CI. The same command runs in three places — locally, in a pre-commit hook, and in CI:

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
(the six paraphrased rules)          {accept,reject}.txt (technical dictionary)
```

### Key Design Principles

The design follows five principles:

- **One command everywhere.** `atmos lint docs [--changed]` runs the same way locally, in the pre-commit hook, and in CI, so no separate code path can drift from what a contributor sees on their own machine.
- **Toolchain-managed installation.** `vale` is a `dependencies.tools` entry on the command, resolved through the `vale-cli/vale` Aqua registry alias, so Atmos installs it automatically wherever the command runs.
- **No copyrighted content.** The rule set and dictionary are original work, inspired by ASD-STE100 but not a copy of it.
- **Non-blocking rollout.** Phase 1 sets every rule to `suggestion` level, so a suggestion never fails a commit, a pre-commit run, or a CI check.
- **Patch-scoped enforcement.** The `--changed` flag lints only files that differ from `origin/main`, the same pattern `atmos lint --changed` already uses for `golangci-lint`.

## Implementation

### 1. Vale Configuration (`.vale.ini`)

`.vale.ini` sets `StylesPath = .vale/styles`, scopes the `Atmos` style to `docs/prd/**/*.md` and `website/docs/**/*.{md,mdx}`, maps the `.mdx` format to `.md` so Vale can parse it, and sets `MinAlertLevel = suggestion`. The `**` selector reaches nested subdirectories too, such as `docs/prd/flag-handling/`.

### 2. Custom Rules (`.vale/styles/Atmos/*.yml`)

Six rules approximate the ASD-STE100 principles we care about most. `Contractions`, `WordyPhrases`, `FutureTense`, and `Terminology` are original work. `PassiveVoice` and `SentenceLength` adapt the irregular-verb list and word-count mechanism from Vale's official Google and Microsoft style packages (MIT-licensed) — see Alternatives Considered for why we borrowed those two pieces but did not adopt either package wholesale.

| Rule | Checks | Approximates |
|---|---|---|
| `PassiveVoice` | Likely passive constructions | ASD-STE100's active-voice rule |
| `Contractions` | Contractions | ASD-STE100's no-contractions rule |
| `WordyPhrases` | Wordy phrases such as "in order to" | ASD-STE100's plain-word rule |
| `SentenceLength` | Sentences over 30 words | ASD-STE100's short-sentence rule |
| `FutureTense` | "will" plus a verb | ASD-STE100's simplest-tense rule |
| `Terminology` | Abbreviations that drift from their full form | ASD-STE100's one-word-one-meaning rule |

### 3. Technical Dictionary

`.vale/styles/config/vocabularies/Atmos/accept.txt` lists the Atmos and infrastructure terms that stay allowed — `Terraform`, `component`, `stack`, and similar words — even though a general-audience dictionary would flag some of them. `reject.txt` lists a short, generic set of words to avoid, unrelated to ASD-STE100's own dictionary.

### 4. Atmos Custom Command

`.atmos.d/lint.yaml` gains a `docs` subcommand, next to the existing `link-check` subcommand:

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

`.atmos.d/toolchain.yaml` gains a `vale: vale-cli/vale` alias, so the command can refer to the tool by its short name.

The command stays a separate subcommand rather than joining the shared `lint changed` pipeline, because that keeps `atmos lint --changed` and `make lint` unchanged for every existing caller while the rule set is still informational. Phase 2 revisits this choice once the rules have proven themselves.

### 5. Pre-Commit Hook

`.pre-commit-config.yaml` gains a local hook, `vale-docs-lint`, that runs `atmos lint docs --changed` on every commit. It mirrors the existing `atmos-validate-editorconfig` hook: a system-language entry that runs on every commit without needing a file list.

### 6. CI Workflow

`.github/workflows/docs-lint.yml` runs that same `atmos lint docs --changed` command inside the `ghcr.io/cloudposse/atmos` container image, on pull requests that touch `docs/prd/`, `website/docs/`, or the Vale configuration. No third-party Vale or reviewdog action is involved — the container already has `atmos`, and the toolchain installs `vale` on demand.

### 7. Skill and Documentation Updates

A new skill, `.claude/skills/writing-style/SKILL.md`, gives an agent-facing summary of the rules and points to `docs/writing-style-guide.md` for detail. `.claude/skills/docs/SKILL.md` gains a "Writing Style" section and an `atmos lint docs --changed` step in its validation checklist. `.claude/skills/changelog/SKILL.md` gains one line drawing the boundary: blog posts follow that skill's narrative rules, not this standard. And `CLAUDE.md` gains a command reference plus a cross-reference from "PRD Documentation (MANDATORY)" to the `writing-style` skill.

## Testing Strategy

Running `atmos lint docs` against the full existing corpus — 1,034 files today — completes with zero errors and zero crashes, which confirms the configuration loads and every rule runs without breaking the scan. Running `atmos lint docs --changed` on a branch with new and modified files confirms the `git diff` scoping picks up exactly those files, and reports "No changed docs to lint" when nothing in scope changed. `pre-commit run vale-docs-lint` confirms the hook executes and reports Passed under the default `suggestion` severity. Direct `vale --config=.vale.ini` runs against sample pages confirm each rule fires on a matching pattern and stays silent otherwise. A blog file produces `Vale.Avoid` findings only, with no `SentenceLength`, `Contractions`, or `PassiveVoice`, which confirms the narrower `website/blog/**` scope.

The one thing we could not test locally is `.github/workflows/docs-lint.yml` itself — a GitHub Actions run needs an actual push to a pull request branch, so this PR documents that limitation rather than skipping it silently.

## Migration Path

### Phase 1 — Ship (this PR)

The tooling lands with `MinAlertLevel = suggestion`, so every finding is informational: nothing blocks, not a local run, not the pre-commit hook, not CI.

### Phase 2 — Promote (future PR)

After a validation window against real pull requests, we promote the rules with the lowest false-positive rate to `warning` or `error`, scoped to changed files. That is what `atmos lint docs --changed` actually does: it selects files with `git diff --name-only` and passes whole files to `vale`, so a promoted rule would flag a pre-existing line in a file the author touched for an unrelated reason. Narrowing enforcement to changed *lines* needs a diff-aware filter that does not exist yet, so it is out of scope for Phase 2 rather than assumed. We will also consider folding `lint docs --changed` into the shared `lint changed` pipeline once the rule set has proven stable.

### Phase 3 — Expand (future PR)

Add further checks as the rule set matures. Tense consistency shipped in v1.0.3 as `FutureTense`. Procedure-step structure remains unbuilt.

Noun-cluster length is the one check we tried and abandoned, and the reason is worth recording so nobody rebuilds it. Recognizing a noun cluster needs part-of-speech tagging, which Vale exposes through its `sequence` extension point. That rule produced 2,407 findings against the corpus, roughly four in five of them wrong, and the `tag` filter that would narrow it had no effect: a sequence restricted to common nouns still matched a line of four proper nouns. An untunable rule at that false-positive rate buries every real finding, which is the same failure `Vale.Terms` produced. Noun clusters stay a human rule until a tagger we can constrain exists.

Existing files stay grandfathered until an author edits them; this PRD does not propose a retroactive rewrite.

## Risks & Mitigation

| Risk | Mitigation |
|---|---|
| Reproducing ASD-STE100's copyrighted specification or dictionary | The rule set and dictionary are original, paraphrased work, and `docs/writing-style-guide.md` links to the official specification instead of copying it. |
| Regex-based heuristics for passive voice and sentence length produce false positives and erode trust | Phase 1 keeps every rule at `suggestion` level, and Phase 2 only promotes severity after measuring the false-positive rate on real PRs. |
| Over a thousand existing files do not comply | The `--changed` flag scopes every check to the current patch, so existing files stay grandfathered until someone edits them, rather than being rewritten wholesale. |
| A contributor's environment cannot reach the Aqua registry to install `vale` | The Atmos toolchain caches the binary after the first successful install, and the command still runs cleanly against zero changed files rather than failing outright. |
| Contributors see the rules as arbitrary | `docs/writing-style-guide.md` explains the reason behind every rule, tied to the ASD-STE100 principle it approximates. |

## Success Criteria

- [ ] `atmos lint docs` completes with zero errors and zero crashes across the existing docs corpus.
- [ ] `atmos lint docs --changed` completes in well under a minute on a typical pull request diff.
- [ ] The pre-commit hook, the CI workflow, and a local run all invoke the identical `atmos lint docs` command.
- [ ] `docs/writing-style-guide.md` documents every active rule and the Atmos technical dictionary.
- [ ] Nothing from the official ASD-STE100 specification or its controlled dictionary appears anywhere in this repository.

## Alternatives Considered

**Rewrite every existing doc up front.** We rejected this: the cost is high, a mechanical rewrite risks introducing factual errors, and it would block unrelated documentation work for months.

**Reproduce the ASD-STE100 dictionary directly, as a Vale spelling style.** We rejected this too, since it would violate ASD's copyright.

**Run a third-party `vale-action` plus `reviewdog` in CI.** We considered this and rejected it, because it adds a dependency outside Atmos's own toolchain and creates a second code path that could drift from the command contributors already run locally. The Atmos-native command, run inside the existing container image, gives the same patch-scoped result with less surface area to maintain.

**Adopt Vale's official Google or Microsoft style packages wholesale, instead of writing our own rules.** We tested this directly: synced both packages and ran them against this PRD. The result was 58 errors, 65 warnings, and 128 suggestions on one file — more than ten times our own rule set's output — and much of it actively worked against our goals rather than toward them. Both packages flag em-dash spacing as a hard error, which would have broken Phase 1's non-blocking design outright. Both flag first-person plural ("we," "our"), which fights the voice this repo's PRDs normally use ("we rejected this because…"). Worse, both packages' `Contractions` rule pushes in the opposite direction from ours — Microsoft enforces contractions as an error, exactly what ASD-STE100 tells us to avoid. We rejected wholesale adoption for these reasons, but we did borrow two things directly. Their (identical) `Passive.yml` has a comprehensive irregular-verb list that catches verbs ours missed, such as "torn" and "given." Microsoft's `SentenceLength.yml` uses an `occurrence`-based word-count mechanism, less fragile than our raw regex, and settles on a better-calibrated 30-word threshold. Both packages are MIT-licensed, so this reuse raises no copyright concern — a different situation from ASD-STE100's own specification.

**Enumerate the rules in the agent-facing skill.** The skill shipped this way in v1.0.0, listing six numbered rules, and we replaced that structure in v1.0.3 after measuring it. Four style instructions went to four fresh agents, each writing the same two documents: a rewrite of a real documentation page, and two PRD sections drafted from a fixed bullet spec. None could read the real skill files or run the linter. We scored the output on `vale` findings and on two signals the linter cannot see — sentence-length standard deviation, and subordinate-connector density — because flattened prose shows up as low values on both.

| Instruction given | Sentence-length stdev | Connectors per 100 words | `vale` per 100 words |
|---|---|---|---|
| Six enumerated rules | 6.4 | 0.64 | 0.21 |
| Standard named, deviations only | 9.0 | 1.76 | 0.78 |
| Deviations plus worked examples | 9.6 | 2.15 | 0.59 |
| No style guidance (control) | 8.3 | 1.35 | 1.16 |
| *A well-written PRD, for reference* | *10.1* | *2.15* | *0.49* |

The enumeration scored best on the linter and worst on everything else. It produced flatter prose than giving no guidance at all, reproducing on demand the exact regression recorded in this document's v1.0.1 entry. The mechanism is that enumerating a partial subset of a standard the reader already knows does not add information — it narrows attention to the listed items, which here were the four mechanically checkable ones, so the writer optimizes those and sacrifices the connected reasoning the standard asks for. Adding worked examples recovered the target voice almost exactly. The skill now names ASD-STE100, documents only where Atmos deviates from it, and carries before/after exemplars.

**Enforce blocking severity from day one.** We rejected this because no prose linting exists today, and a sudden blocking gate would fail most open pull requests and create avoidable churn. The phased Migration Path above exists for exactly this reason.

## References

- [ASD-STE100 home page](https://www.asd-ste100.org/) — the authoritative specification.
- [Vale documentation](https://vale.sh/docs/) — the linter this PRD adopts.
- `docs/writing-style-guide.md` — the full rule list and technical dictionary.
- `.claude/skills/writing-style/SKILL.md` — the agent-facing skill.
- `.atmos.d/lint.yaml` — the `atmos lint docs` custom command.

## Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0.0 | 2026-07-31 | Initial adoption: Vale configuration, custom rules, technical dictionary, `atmos lint docs` command, pre-commit hook, CI workflow, `writing-style` skill, and style guide. |
| 1.0.1 | 2026-07-31 | Rewrote the prose: restored natural sentence connectors (because, so, since) that a linter-driven editing pass had stripped out, producing flat, disconnected sentences. ASD-STE100 controls sentence length and vocabulary; it does not forbid ordinary subordinate clauses. |
| 1.0.2 | 2026-07-31 | Softened the `PassiveVoice` and `SentenceLength` rule messages to invite judgment instead of demanding mechanical compliance; tested Vale's official Google and Microsoft style packages against this PRD and adapted their `Passive.yml` verb list and `SentenceLength.yml` mechanism, but rejected wholesale adoption (see Alternatives Considered). |
| 1.0.3 | 2026-07-31 | Recalibration after measuring the shipped Phase 1 against the corpus. Removed 1,618 false positives: a bare `#` in `reject.txt` that Vale compiled into a live pattern matching every hash in the docs (30 hits), and `Vale.Terms`, which flagged `yaml`/`json` in code fences and environment variable names such as `KUBECONFIG` as miscased prose (1,588 hits). Re-tightened the `PassiveVoice` guidance, which v1.0.2 had over-softened: the flattening incident came from splitting sentences, not from active voice. Added `FutureTense` and `Terminology`. Built and rejected a `NounClusters` rule (Vale's `sequence` tag filter has no effect; 2,407 findings at roughly 80% false positive). Extended `Vale.Avoid` alone to `website/blog/**`. Replaced the `writing-style` skill's six-rule enumeration with an anchor-plus-exemplars structure after a controlled test showed the enumeration produced flatter prose than giving no guidance at all. Fixed the `--changed` pathspec, which silently skipped two top-level `website/docs/*.mdx` files. |
