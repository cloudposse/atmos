# Atmos Writing Style Guide

This guide sets the writing style for Atmos PRDs (`docs/prd/`) and website
documentation (`website/docs/`). It does not apply to `website/blog/`
(changelog posts), which use the narrative style owned by the `changelog`
skill.

The style is inspired by **ASD-STE100** (Simplified Technical English), the
controlled-language standard the aerospace industry uses for maintenance
manuals. ASD-STE100 exists to remove ambiguity: one sentence, one meaning,
readable by people who do not speak English as a first language.

## About ASD-STE100

The official ASD-STE100 specification and its controlled dictionary of
approved words are copyrighted by ASD (Brussels) and cannot be reproduced
here. This guide is our own paraphrase of its writing principles, adapted for
software documentation. Read the full specification at
[asd-ste100.org](https://www.asd-ste100.org/) if you want the complete
standard. See `docs/prd/technical-writing-standard-ste100.md` for why we
adopted this approach instead of copying the specification.

## Rules we check automatically

Run `atmos lint docs` (or `atmos lint docs --changed` for a patch-scoped
check) to run these rules with `vale`. Every rule is `suggestion` level today
(Phase 1 — see the PRD's Migration Path): a suggestion never fails a build,
a commit, or a CI check.

| Rule | What it flags | Why |
|---|---|---|
| `Atmos.SentenceLength` | Sentences longer than ~25 words | A short sentence has one meaning. A long sentence hides its meaning in clauses. |
| `Atmos.PassiveVoice` | Likely passive constructions ("is used", "was created") | Active voice names the actor. The reader always knows who or what does the action. |
| `Atmos.Contractions` | Contractions ("don't", "it's") | An expanded word ("do not") cannot be misread. A contraction can. |
| `Atmos.WordyPhrases` | Wordy phrases ("in order to", "utilize") | A short, common word is faster to read and easier to translate than a long one. |

These four rules are a starting set, not the full standard. Expect the rule
set to grow as Phase 2 and Phase 3 of the rollout add more checks.

## A `vale` finding is a prompt, not a mandate

Write naturally first. Run `atmos lint docs` afterward, as a spot check —
not as a loop you edit against sentence by sentence until every finding
clears. Writing to satisfy a linter, one finding at a time, produces flat,
disconnected prose: short declarative sentences with no "because," "so," or
"which" between them, because those words add length or (in the case of
"which") look like the start of a forbidden clause. That is not what
ASD-STE100 asks for. Real Simplified Technical English still reads as
connected reasoning — it just avoids ambiguity and unnecessary complexity.

Two rules need real judgment, not mechanical compliance, more than the
others:

- **`Atmos.SentenceLength` is a rough proxy, not the rule.** The actual
  rule is one idea per sentence. A 30-word sentence that states one
  connected idea, joined by plain connectors, usually reads more clearly
  than the same idea chopped into three fragments. Split a flagged
  sentence only if it genuinely holds more than one idea — never just to
  clear the finding.
- **Passive voice is sometimes the right choice.** "No third-party action
  is involved" reads better than any active rewrite, because the actor
  ("nobody") is not the point of the sentence. Rewrite a flagged passive
  construction only when naming the actor actually removes ambiguity.
  Inventing an actor just to satisfy the rule ("the workflow does not
  involve a third-party action") usually reads worse, not better.

If a finding does not hold up to that judgment, leave it. `suggestion`
severity means exactly that: a suggestion.

## Rules to apply by hand

`vale` cannot check everything ASD-STE100 recommends. Until we add automated
checks for these, apply them yourself when you write or review a PRD or a
docs page:

- **One word, one meaning.** Do not use the same word for two different
  ideas, and do not use two different words for the same idea. Pick one term
  per concept and use it every time (for example, always "component", never
  alternating with "module" or "resource" for the same thing).
- **Write one instruction per step.** In a numbered procedure, each step does
  one thing. Split a step that contains "and then" into two steps.
- **Put a warning or caution before the step it applies to**, never after.
- **Avoid long noun strings.** "The stack config validation error handler"
  is hard to parse. Break it up: "the handler that processes validation
  errors in the stack config."
- **Use articles.** Write "the component," not "component," except in
  headings, tables, and code.
- **Prefer the simplest tense.** Use present tense for facts and
  instructions ("Atmos writes the file," not "Atmos will write the file").

## The Atmos technical dictionary

ASD-STE100 allows technical nouns that a general-audience dictionary would
flag as too complex, as long as the domain requires them ("turbine," in
aviation maintenance). Atmos documentation follows the same allowance: proper
nouns and domain terms like "Terraform," "OpenTofu," "component," "stack,"
"toolchain," and "vendor" are always permitted, even though they are not
everyday words.

The full accepted list lives in
`.vale/styles/config/vocabularies/Atmos/accept.txt`; the list of words we
prefer to avoid, with reasons, lives in
`.vale/styles/config/vocabularies/Atmos/reject.txt`. Add a term to
`accept.txt` when a new domain word is genuinely needed and would otherwise
get flagged.

## Scope

| Content | In scope | Style owner |
|---|---|---|
| `docs/prd/**` | Yes | This guide |
| `website/docs/**` | Yes | This guide |
| `website/blog/**` | No | `changelog` skill |

## References

- [ASD-STE100 home page](https://www.asd-ste100.org/) — the authoritative
  specification.
- [Vale documentation](https://vale.sh/docs/) — the linter that enforces
  this guide.
- `docs/prd/technical-writing-standard-ste100.md` — the PRD that adopted
  this standard, including the full rollout plan.
- `.claude/skills/writing-style/SKILL.md` — the agent-facing skill that
  wraps this guide.
