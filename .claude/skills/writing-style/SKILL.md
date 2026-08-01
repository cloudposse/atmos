---
name: writing-style
description: "Atmos writing style for PRDs and website docs, based on ASD-STE100 (Simplified Technical English) principles: short sentences, active voice, no contractions, plain words, and the atmos lint docs vale check. Invoke when writing or reviewing a PRD (docs/prd/) or a website/docs page, or when a docs-lint (vale) finding needs interpreting. Not for website/blog posts — use the changelog skill instead."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Writing Style

Use this skill when you write or review a PRD (`docs/prd/`) or a page under
`website/docs/`. It does not apply to `website/blog/` — blog posts are
narrative content owned by the `changelog` skill.

## The standard

Atmos PRDs and docs follow a writing style inspired by **ASD-STE100**
(Simplified Technical English). The full rule list, rationale, and the Atmos
technical dictionary live in `docs/writing-style-guide.md` — read it before
writing a PRD or a substantial docs page. This skill is the short,
agent-facing summary.

## Core rules

1. **One idea per sentence.** 20-25 words is a rough guide, not a hard cap. Split a sentence only when it genuinely holds more than one idea — not merely because it contains "and," "which," "so," or "because." Those are normal connectors; removing them is what produces flat, disconnected prose.
2. **Active voice, unless the actor doesn't matter.** Name the actor when it adds clarity: "Atmos writes the file," not "the file is written." Leave a passive construction alone when naming the actor would be forced or irrelevant ("no third-party action is involved" is fine as written).
3. **No contractions.** Write "do not," not "don't."
4. **Plain words over wordy phrases.** "To," not "in order to." "Use," not "utilize."
5. **One word, one meaning.** Pick one term per concept and use it consistently — do not alternate between synonyms for the same thing.
6. **One instruction per step** in a numbered procedure.

Write naturally first, using these rules as a mental checklist while drafting. Run `atmos lint docs` afterward as a spot check, not as a loop you edit against sentence by sentence — see "Treat findings as prompts" below.

## Checking your writing

```shell
atmos lint docs            # lint every PRD and docs page
atmos lint docs --changed  # lint only files changed from origin/main
```

This runs `vale` against `.vale.ini` and the custom rules in
`.vale/styles/Atmos/`. The Atmos toolchain installs `vale` automatically —
no manual setup. The same command runs locally, in the pre-commit hook
(`vale-docs-lint`), and in CI (`.github/workflows/docs-lint.yml`, inside the
Atmos container image).

**Phase 1 (current):** every rule is `suggestion` level
(`.vale.ini` `MinAlertLevel = suggestion`). Findings are informational and
never fail a commit, a pre-commit run, or a CI check. Treat a suggestion as a
prompt to consider a rewrite, not a hard requirement — especially in existing
docs that predate this standard (they are grandfathered until edited; see the
Migration Path in `docs/prd/technical-writing-standard-ste100.md`).

### Treat findings as prompts, not mandates

Do not rewrite every flagged sentence just to clear the finding. Read each
one and decide:

- **`Atmos.SentenceLength`** flags length as a proxy for "more than one
  idea." If a long sentence is genuinely one connected idea, leave it —
  the word count alone is not a defect.
- **`Atmos.PassiveVoice`** flags a pattern, not a verdict. If naming the
  actor would be forced, or the actor is unknown or beside the point,
  the passive form may already be the clearest option.

A PRD or docs page that clears every single finding by mechanically
rewriting each one usually reads worse, not better — see
`docs/writing-style-guide.md`'s "A `vale` finding is a prompt, not a
mandate" section for the fuller version of this guidance.

## When a new domain term gets flagged

If `vale` flags a legitimate Atmos or infrastructure term (for example, a new
component type name), add it to
`.vale/styles/config/vocabularies/Atmos/accept.txt` rather than rewording
around it. See `docs/writing-style-guide.md` for the rationale.

## Related skills

| Need | Skill |
|---|---|
| PRD structure, placement, naming | none dedicated — see `CLAUDE.md`'s "PRD Documentation" section and `docs/prd/` for examples |
| Website docs conventions (sidebar, front matter, screengrabs) | `docs` |
| Blog post style (narrative, problem-first framing) | `changelog` |
| Roadmap page updates | `roadmap` |
