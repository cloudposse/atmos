# Atmos Writing Style Guide

This guide sets the writing style for Atmos PRDs (`docs/prd/`) and website
documentation (`website/docs/`). It does not apply to `website/blog/`
(changelog posts), which use the narrative style owned by the `changelog`
skill.

Atmos uses a writing style **inspired by ASD-STE100** (Simplified Technical
English), the controlled-language standard the aerospace industry uses for
maintenance manuals. ASD-STE100 exists to remove ambiguity: one sentence, one
meaning, readable by people who do not speak English as a first language.

The Atmos style is our own work, adapted for software documentation. Calling a
document "ASD-STE100" would claim conformance to a published specification that
we neither reproduce nor verify against, so this guide does not make that claim.
It borrows the principles and says where it departs from them.

## About ASD-STE100

The official ASD-STE100 specification and its controlled dictionary of
approved words are copyrighted by ASD (Brussels) and cannot be reproduced
here. This guide is our own paraphrase of its writing principles, adapted for
software documentation. Read the full specification at
[asd-ste100.org](https://www.asd-ste100.org/) if you want the complete
standard. See `docs/prd/technical-writing-standard-ste100.md` for why we
adopted this approach instead of copying the specification.

## How to apply it

Apply the standard the way a technical writer would, as a way of thinking
about clarity, rather than as a checklist to run against each sentence.
Most writers, and every current language model, already know what Simplified
Technical English asks for. This guide therefore documents where Atmos
differs from a strict reading, and where a naive reading goes wrong. It does
not restate the standard.

That choice is measured, not stylistic. See "Why this guide does not
enumerate rules" below.

### Where Atmos differs from a strict reading

- **Keep ordinary subordinate connectors.** "Because," "so," "since," and
  "which" are normal English and STE does not ban them. Prose that removes
  them reads as a list of disconnected fragments. Connected reasoning is the
  goal, not clipped sentences.
- **Sentence length is a smell, not a limit.** One connected idea at 40 words
  beats the same idea chopped into three fragments. Split for a second idea,
  never for a word count.
- **Active voice gets no such latitude.** Passive is the easiest thing to
  write by accident and it hides the actor. Rewrite it by default. Keep the
  passive form only when the actor is genuinely unknown or genuinely beside
  the point.
- **First person plural is fine in PRD rationale** ("we rejected this
  because…") and is avoided in `website/docs/` reference pages, which address
  the reader directly instead.
- **There is no controlled dictionary.** Atmos does not reproduce ASD's
  approved word list, so domain and technical nouns are unrestricted.
- **Present tense for facts and instructions.** "Atmos writes the file," not
  "Atmos will write the file."
- **No contractions.** Write the full form of every verb pair.
- **Never open prose with an inline code span.** Start a sentence or
  paragraph with a word. Bullets may open with a backtick.
- **Voice is engineering-peer: dry, specific, concrete.** No marketing
  adjectives.

### Rules that still need a human

`vale` cannot check these. Apply them yourself when you write or review:

- **Avoid long noun strings.** "The stack config validation error handler"
  is hard to parse. Break it up: "the handler that processes validation
  errors in the stack config." No linter checks this; see the note under
  "Rules we check automatically" for why.
- **One instruction per step.** In a numbered procedure, each step does one
  thing. Split a step that contains "and then" into two steps.
- **Put a warning or caution before the step it applies to**, never after.
- **Use articles.** Write "the component," not "component," except in
  headings, tables, and code.

## Rules we check automatically

Run `atmos lint docs` (or `atmos lint docs --changed` for a patch-scoped
check) to run these rules with `vale`. Every rule is `suggestion` level today
(Phase 1 — see the PRD's Migration Path): a suggestion never fails a build,
a commit, or a CI check.

| Rule | What it flags | Trust it? |
|---|---|---|
| `Atmos.PassiveVoice` | Likely passive constructions ("is used", "was created") | Yes — this rule carries the most weight |
| `Atmos.Contractions` | Contractions | Yes — almost no false positives |
| `Atmos.WordyPhrases` | Wordy phrases ("in order to", "utilize") | Yes |
| `Atmos.FutureTense` | "will" plus a verb, where present tense reads better | Yes |
| `Atmos.Terminology` | Abbreviations that drift ("repo" for "repository") | Mostly — about a fifth of hits sit in inline code |
| `Vale.Avoid` | Marketing filler ("leverage", "seamlessly", "robust") | Yes |
| `Atmos.SentenceLength` | Sentences longer than 30 words | Advisory only — see below |

`Vale.Terms` and `Vale.Spelling` are switched off. `Vale.Terms` enforces the
casing of every `accept.txt` entry, which produced 1,588 findings against the
current corpus and every one was a false positive: `yaml` and `json` inside
code fences and config keys, `cli` inside documentation paths, and
environment variable names such as `KUBECONFIG` reported as miscased prose.

There is no automated noun-cluster check, and the reason is worth recording
so nobody rebuilds it. Recognizing a noun cluster needs part-of-speech
tagging, which Vale exposes through its `sequence` extension point. We built
that rule and measured it: 2,407 findings against the corpus, of which
roughly four in five were wrong, and the `tag` filter that would narrow them
had no effect — a sequence restricted to common nouns still matched a line of
four proper nouns. An untunable rule at that false-positive rate would bury
every real finding, which is the same failure `Vale.Terms` produced. Noun
clusters stay a human rule until a tagger we can actually constrain exists.

## A `vale` finding is a prompt, not a mandate

Write naturally first. Run `atmos lint docs` afterward, as a spot check —
not as a loop you edit against sentence by sentence until every finding
clears. Writing to satisfy a linter, one finding at a time, produces flat,
disconnected prose: short declarative sentences with no "because," "so," or
"which" between them, because those words add length or (in the case of
"which") look like the start of a forbidden clause. That is not what
ASD-STE100 asks for. Real Simplified Technical English still reads as
connected reasoning — it just avoids ambiguity and unnecessary complexity.

One rule needs real judgment rather than mechanical compliance:

- **`Atmos.SentenceLength` is a rough proxy, not the rule.** The actual
  rule is one idea per sentence. A 30-word sentence that states one
  connected idea, joined by plain connectors, usually reads more clearly
  than the same idea chopped into three fragments. Split a flagged
  sentence only if it genuinely holds more than one idea — never just to
  clear the finding.

**`Atmos.PassiveVoice` is different, and it is worth being explicit about
why.** An earlier version of this guide grouped the two rules together and
told you to use judgment on both. That was the wrong lesson drawn from a
real incident: a linter-driven editing pass flattened this standard's own
PRD, and the damage came from splitting sentences, not from writing in the
active voice. Sentence length is where the latitude belongs.

Passive voice is the easiest thing on this list to write by accident, and
it hides the actor — the exact ambiguity the style exists to remove. So
rewrite a flagged passive by default. The exception is real but narrow:
"No third-party action is involved" reads better than any active rewrite,
because the actor ("nobody") is not the point of the sentence. Keep the
passive form when the actor is genuinely unknown or genuinely beside the
point, and never invent an actor just to satisfy the rule ("the workflow
does not involve a third-party action" reads worse, not better). That
exception covers a minority of findings.

Where a finding genuinely does not hold up, leave it. `suggestion`
severity means exactly that: a suggestion.

## Why this guide does not enumerate rules

An earlier version of the `writing-style` skill listed six numbered rules.
We replaced that structure after testing it, because it measurably produced
worse writing than the alternatives.

The test gave four different style instructions to four fresh agents, each
writing the same two documents: a rewrite of a real documentation page and
two PRD sections drafted from a fixed bullet spec. Neither the linter nor the
real skill files were available to them. We scored the output on `vale`
findings and on two signals the linter cannot see — sentence-length standard
deviation, and how densely the prose uses subordinate connectors. Flattened
writing shows up as low values on both.

| Instruction given | Sentence-length stdev | Connectors per 100 words | `vale` per 100 words |
|---|---|---|---|
| Six enumerated rules | 6.4 | 0.64 | 0.21 |
| Standard named, deviations only | 9.0 | 1.76 | 0.78 |
| Deviations plus worked examples | 9.6 | 2.15 | 0.59 |
| No style guidance (control) | 8.3 | 1.35 | 1.16 |
| *A well-written PRD, for reference* | *10.1* | *2.15* | *0.49* |

The enumerated rules scored best on the linter and worst on everything else.
Connector density fell to 0.64 per 100 words against the reference PRD's
2.15, and mean sentence length collapsed from 21.7 words to 14.2. That output
was flatter than the output from giving no guidance at all, which reproduces
the exact regression that forced the PRD rewrite recorded in its v1.0.1
changelog entry.

The reason is worth keeping in mind whenever this guide grows. Enumerating a
partial subset of a standard that the reader already knows does not add
information — it narrows their attention to the listed items, which here were
the four mechanically checkable ones. Writers then optimize those four and
sacrifice the connected reasoning the standard actually asks for. Adding
worked examples recovered the target voice almost exactly.

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

Never leave a bare `#` on a line of its own in either file. Vale strips a
comment only when the `#` is followed by text, so a lone `#` compiles into a
live pattern that matches every literal hash in the corpus. Separate
paragraphs with a blank line instead.

## Scope

| Content | In scope | Style owner |
|---|---|---|
| `docs/prd/**` | Yes | This guide |
| `website/docs/**` | Yes | This guide |
| `website/blog/**` | Narrative style, plus `Vale.Avoid` | `changelog` skill |

## References

- [ASD-STE100 home page](https://www.asd-ste100.org/) — the authoritative
  specification.
- [Vale documentation](https://vale.sh/docs/) — the linter that enforces
  this guide.
- `docs/prd/technical-writing-standard-ste100.md` — the PRD that adopted
  this standard, including the full rollout plan.
- `.claude/skills/writing-style/SKILL.md` — the agent-facing skill that
  wraps this guide.
