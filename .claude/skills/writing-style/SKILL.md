---
name: writing-style
description: "Atmos writing style for PRDs and website docs, inspired by ASD-STE100 (Simplified Technical English): active voice, one idea per sentence, plain words, no contractions, and the atmos lint docs vale check. Invoke when writing or reviewing a PRD (docs/prd/) or a website/docs page, or when a docs-lint (vale) finding needs interpreting. Not for website/blog posts — use the changelog skill instead."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "2.0.0"
---

# Writing Style

Atmos PRDs (`docs/prd/`) and website documentation (`website/docs/`) follow a writing
style **inspired by ASD-STE100 (Simplified Technical English)**, adapted for software
documentation. The style is Atmos's own; calling a document "ASD-STE100" would claim
conformance to a specification we neither reproduce nor verify against. This skill does
not apply to `website/blog/` — blog posts are narrative content owned by the `changelog`
skill.

Apply those principles the way a technical writer would: as a way of thinking about
clarity, not as a checklist to run against each sentence. You already know what Simplified
Technical English asks for. What follows is only where Atmos departs from it, or where a
naive reading of it goes wrong.

This is deliberate. An earlier version of this skill enumerated six rules instead, and a
controlled test found that framing produced the *flattest* prose of any variant tested —
worse than giving no style guidance at all. Restating a lossy subset of a standard you
already know narrows it rather than adding to it. `docs/writing-style-guide.md` records
the measurements.

## Where Atmos differs from a strict reading

- **Keep ordinary subordinate connectors.** "Because," "so," "since," and "which" are
  normal English and STE does not ban them. Prose that removes them reads as a list of
  disconnected fragments. Connected reasoning is the goal, not clipped sentences.
- **Sentence length is a smell, not a limit.** One connected idea at 40 words beats the
  same idea chopped into three fragments. Split for a second idea, never for a word count.
- **Active voice gets no such latitude.** Passive is the easiest thing to write by
  accident and it hides the actor. Rewrite it by default. Keep the passive form only when
  the actor is genuinely unknown or genuinely beside the point.
- **First person plural is fine in PRD rationale** ("we rejected this because…") and is
  avoided in `website/docs/` reference pages, which address the reader directly instead.
- **There is no controlled dictionary.** Atmos does not reproduce ASD's approved word
  list, so domain and technical nouns are unrestricted. Write "idempotent," "vendoring,"
  and "OIDC" freely.
- **Present tense for facts and instructions.** "Atmos writes the file," not "Atmos will
  write the file." Future tense is almost always avoidable in reference documentation.
- **No contractions.** Write the full form of every verb pair.
- **Never open prose with an inline code span.** Start a sentence or paragraph with a
  word. Bullets may open with a backtick.
- **Voice is engineering-peer: dry, specific, concrete.** No marketing adjectives.
  Avoid "leverage," "seamlessly," "robust," "simply," and "obviously" entirely.

## Worked examples

These are the patterns that matter most, drawn from real Atmos documentation.

**Passive hiding the actor — rewrite it.**

> Before: The varfile is generated from the stack configuration and is written to the
> component directory.
>
> After: Atmos generates the varfile from the stack configuration and writes it to the
> component directory.

The actor is knowable and load-bearing. Naming it removes the ambiguity.

**Passive where the actor is genuinely irrelevant — leave it.**

> Keep as written: No third-party action is involved.

The actor here is "nobody." An active rewrite ("the workflow does not involve a
third-party action") is longer and reads worse. This case is a minority of findings, not
a general excuse.

**Future tense in reference prose — use present.**

> Before: A major release will be published when there is a breaking change. Several
> release candidates will be published prior to a major release.
>
> After: A major release follows any breaking change. Release candidates come first, so
> the change gets feedback before it ships.

Fixing the tense also removed the passive and the wordy "prior to." These defects travel
together, which is why editing for one rule at a time works badly.

**Noun cluster — break it up.**

> Before: the stack config validation error handler
>
> After: the handler that processes validation errors in the stack config

**Over-application — the failure to avoid.** This standard's own PRD was once edited
sentence by sentence against the linter until every finding cleared. The result:

> Atmos has around 218 PRDs. It has around 815 documentation pages. Many authors wrote
> them. They were written over several years. Sentence length varies. Voice varies. Word
> choice varies.

That clears every rule and is worse writing. What it should say, and now does:

> Atmos has around 218 PRDs in `docs/prd/` and around 815 pages in `website/docs/`,
> written by many different authors over several years, so sentence length, voice, and
> word choice vary widely from one document to the next.

One connected idea, plain connectors, 34 words. The linter flags it. The linter is wrong
here, and that is the expected and acceptable outcome.

## Checking your work

```shell
atmos lint docs            # lint every PRD and docs page
atmos lint docs --changed  # lint only files changed from origin/main
```

Write naturally first, then run the linter as a spot check. Do not edit against it
sentence by sentence until every finding clears. That produces flat prose and it is a
mistake this project has already made once.

This runs `vale` against `.vale.ini` and the custom rules in `.vale/styles/Atmos/`. The
Atmos toolchain installs `vale` automatically. The same command runs locally, in the
pre-commit hook (`vale-docs-lint`), and in CI (`.github/workflows/docs-lint.yml`).

**Phase 1 (current):** every rule is `suggestion` level, so findings never fail a commit,
a pre-commit run, or a CI check. Existing documents are grandfathered until someone edits
them; see the Migration Path in `docs/prd/technical-writing-standard-ste100.md`.

Which findings to trust:

- **`Atmos.PassiveVoice`** — fix it. This is the rule that carries the most weight, and
  the narrow exception above covers a minority of hits, not a general license to skip.
- **`Atmos.Contractions`**, **`Atmos.WordyPhrases`**, **`Atmos.FutureTense`**,
  **`Vale.Avoid`** — fix these. They have almost no false positives.
- **`Atmos.Terminology`** — mostly right. About a fifth of its hits fall inside inline
  code spans, where the abbreviation is a real identifier and must stay.
- **`Atmos.SentenceLength`** — advisory. It flags at 30 words as a proxy for "more than
  one idea." A long sentence carrying one connected idea is fine as written.

Noun clusters are not checked by any rule; that one is on you.

## When a new domain term gets flagged

If `vale` flags a legitimate Atmos or infrastructure term, add it to
`.vale/styles/config/vocabularies/Atmos/accept.txt` rather than rewording around it.
Do not leave a bare `#` on its own line in that file or in `reject.txt`; Vale compiles it
into a live pattern that matches every hash in the corpus.

## Related skills

| Need | Skill |
|---|---|
| PRD structure, placement, naming | none dedicated — see `CLAUDE.md`'s "PRD Documentation" section and `docs/prd/` for examples |
| Website docs conventions (sidebar, front matter, screengrabs) | `docs` |
| Blog post style (narrative, problem-first framing) | `changelog` |
| Roadmap page updates | `roadmap` |
