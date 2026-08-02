---
name: changelog
description: "Blog post authoring for Atmos: MDX template, frontmatter, website/blog/tags.yml and authors.yml rules, problem-first framing, backtick-opening ban, optional cast embeds, and no-Go-internals leakage. Invoke when writing, editing, or reviewing a website/blog/*.mdx changelog post."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
---

# Changelog (Blog Post) Authoring

Use this skill whenever you create or edit a post under `website/blog/`. It is the single source of truth
for the template, tags, authors, and style rules — `CLAUDE.md`, the `pull-request` skill, and the `docs`
skill all point here instead of restating these rules. Don't re-duplicate them elsewhere.

Blog posts are narrative, reader-facing content — the rules in this skill (problem-first framing, etc.)
govern them, not the `writing-style` skill's ASD-STE100 rules, which apply to PRDs and `website/docs/` only.
Long sentences, contractions, and the occasional passive are part of the house voice here, and nothing
enforces against them: `.vale.ini`'s `website/blog/**` sections deliberately omit the `Atmos` style.

One check does carry over. `atmos lint docs` runs `Vale.Avoid` against blog posts, which flags marketing
filler — "leverage", "synergy", "seamlessly", "robust", "simply", "obviously". That is a house-voice rule
everywhere, and it arguably matters more here than in a PRD, because the blog is the surface where
marketing language creeps in. Running `atmos lint docs --changed` on a post is safe and worth doing.

Two more rules in this skill are house-voice rules the `writing-style` skill also applies to docs, so keep
them in mind if you move between the two: never open prose with an inline code span (Rule 2 below), and
keep the voice engineering-peer — dry, specific, no marketing adjectives.

## When a post is required

Only non-draft PRs targeting `main`, labeled `minor` or `major`, need one — see the `pull-request` skill's
label decision tree. CI enforces this via `.github/workflows/changelog-check.yml`, which checks for a new
`website/blog/*.md` or `*.mdx` file (draft PRs, and PRs targeting a branch other than `main`, are exempt
entirely). Write posts as `.mdx` regardless — Rule 3 below embeds `<CastPlayer>` as real JSX, which only
`.mdx` renders; CI accepts `.md` but that's not this repo's convention.
If a change is genuinely internal-only with zero user-visible effect, it doesn't get a post at all — that
invariant belongs to the `roadmap` skill ("no changelog post for internal-only refactors"); don't work around
it by writing an implementation-heavy post instead.

## File and frontmatter

Create `website/blog/YYYY-MM-DD-<slug>.mdx`:

```markdown
---
slug: descriptive-slug
title: "Clear Title"
authors: [username]
tags: [feature]
date: YYYY-MM-DDT12:00:00.000Z
---
Open on the PAIN the reader already feels — the broken/tedious/confusing thing they live with
today — then name the change as the relief.
<!--truncate-->
## The Problem
...
## The Fix
...
## How to Use It
...
## Get Involved
```

- `.mdx`, YAML frontmatter, `<!--truncate-->` immediately after the intro paragraph(s) — that's what shows in
  the blog feed.
- `date:` carries the full ISO timestamp at noon UTC, matching the date in the filename. Nearly every recent
  post sets it. Docusaurus infers the date from the filename without it, but the explicit field is the
  convention and it keeps same-day posts from ordering arbitrarily.
- Never open the body with `## What Changed` — lead with the problem (see Rule 1).
- `## Get Involved` is one or two sentences ending in a link to
  `https://github.com/cloudposse/atmos/issues`. Keep it that short — it is a sign-off, not a section.

## Tags — read `website/blog/tags.yml`, never invent one

User-facing: `feature`, `enhancement`, `bugfix`, `dx`, `breaking-change`, `security`, `documentation`,
`deprecation`, `experimental`, `atmos-pro`. Internal/contributor-only, zero user impact: `core`.

## Authors — read `website/blog/authors.yml`

Use the individual human contributor's GitHub username, not a generic team byline. This repo's own history
favors real usernames overwhelmingly (e.g. `osterman` and `aknysh` account for the large majority of posts) —
a generic `atmos` author appears on only a small minority of posts and is a pattern to avoid going forward.
**If the contributor isn't in `authors.yml` yet, add them in the same PR** before referencing their username
in frontmatter.

## Rule 1 — Problem-first framing (not feature-first)

The intro (the text above `<!--truncate-->`) must open on the reader's pain, not on what Atmos now does.
Don't make the post self-referential ("Atmos doesn't support X, so we added it") — describe the general
problem or technique first, the way someone outside the project would recognize it, then bring in the fix.

- **Violation** — "Atmos now builds itself through a first-class Atmos command:" — self-referential and
  feature-first. Rewritten to open on the pain instead: "Every long-lived repository ends up with a Makefile
  that only a couple of people fully understand..."
- **Violation** — "The new `atmos list dependencies` command renders..." — feature-first. Rewritten to:
  "Every infrastructure repository encodes a dependency graph, whether or not anyone ever writes it down..."
- **Correct** — "A workflow fails in CI. You open the run and you're staring at two thousand lines of
  undifferentiated output..." — pain first, product named later.
- **Correct** — "Package managers normally let you discover available updates, review their impact, and
  apply them locally. Vendored Atmos components did not:..." — names the general technique, then the gap.
- **Correct pattern for a hypothetical vendoring feature**, illustrating the same principle: don't write
  "Atmos doesn't support vendoring, so we added it." Instead: "Projects depend on lots of external artifacts.
  Vendoring is a common technique to bring those into the repo so changes to dependencies aren't opaque. It's
  also supportive of immutable infrastructure." — name the general problem/technique, then the fix.

Structure the body `## The Problem` / `## The Fix` / `## How to Use It` / `## Get Involved`.

## Rule 2 — Never open prose with a backtick

Prose (a sentence, paragraph, or the post intro) must start with a word, not an inline code span or fence.
**Bullets may open with a backtick** — this rule is about prose paragraphs only.

The rule reaches **every sentence**, not just the first one in a paragraph. A code span opening a
paragraph's second or third sentence is the same violation.

- **Violation** — "`atmos auth list` solves these challenges..."
- **Violation** — "`atmos git clone` is Atmos's native replacement for..."
- **Violation** — "`action: build` now accepts `driver` and `cache`:"
- **Fix pattern**: lead with a word, then the code span. "The `atmos auth list` command solves these
  challenges...", "The `action: build` step now accepts `driver` and `cache`:"

## Rule 3 — Cast embedding (optional, preferred when a recording exists)

Only a small minority of recent posts embed a cast — it's a nice-to-have, not a requirement, and should never
block a post.

Embed only when a recording demonstrates **the change this post announces**. A cast that exists for the same
subsystem but a different feature is worse than no cast, because it shows the reader something other than what
they just read about. "A recording exists" is not the trigger; "a recording shows this" is.

When such a recording exists (or is worth recording) under `examples/<name>/` or `demo/casts/...` per the
`atmos-asciicast` skill, embed it near the top of the post, after the intro/truncate:

```mdx
import CastPlayer from '@site/src/components/CastPlayer'

<CastPlayer src="/casts/examples/demo-component-versions/vendor-versions.cast" title="atmos component version vendoring" chrome controls scrubber />
```

- `src` points under `website/static/casts/{examples,demo}/...`.
- Always carry the `chrome controls scrubber` flags.
- Multiple `<CastPlayer>` tags are fine in one post if there are multiple relevant recordings.
- Follow it with a plain link to the full example when one exists: `[View the full example](/examples/<name>)`.
- Don't use `EmbedExample` in blog posts — that component's README/file-listing duplicates content the post's
  own prose already covers; it's for docs pages that need the "browse the full example" callout instead.

## Rule 4 — No Go / implementation-detail leakage

A blog post is for users, not contributors. Never name Go package paths, internal file layout, or
implementation structure — describe behavior only in CLI/config/output terms.

**The title is in scope.** It is the highest-impact place internals leak, and retitling is safe — the URL
comes from `slug`, not the title.

- **Violation** — a post titled "New pkg/function Package for Format-Agnostic Function Registry", whose body
  named `pkg/function/`, `pkg/yaml/`, and `pkg/aws/identity/`. A business reader does not care about Go
  package paths. Retitled to "One Registry Behind Every Atmos Function", with the body re-expressed as
  behavior: "every Atmos function now resolves through one registry that knows nothing about the format the
  function was written in."
- **Correct** — posts that describe mechanisms only in terms of commands, flags, and observable output.

**If removing the internals leaves nothing to say**, the change was genuinely internal and the honest move is
to state the user-facing consequence plainly — "Nothing changes in your stacks" — and then give the reader the
one thing that *is* useful to know. Do not pad the post to fill the four-section template.

## Editing an already-published post

Everything above is written for authoring a new post. Retrofitting an existing one has different rules,
because the post already has readers and a URL:

- **Never change `slug`, `date`, or the filename.** Those are the published URL. The `title` is fair game.
- **Change only what a rule actually requires.** A post that already complies is a valid outcome; leave it
  alone rather than finding something to improve.
- **The four-section structure is the spine, not a fence.** Problem → Fix → How to Use It → Get Involved must
  be present and in that order. An extra section between them is fine if it carries real content. When a
  post is missing one of the four, prefer moving existing prose into it over writing new prose to fill it —
  a thin invented "The Problem" section is worse than the heading being absent.
- **Do not trade away facts to satisfy a rule.** If Rule 4 forces you to drop implementation detail, keep
  every fact that has a user-observable analogue and drop only the internals themselves.

## Pre-publish checklist

- [ ] Intro opens on the problem, not the feature, and doesn't open with a backtick
- [ ] Body follows Problem → Fix → How to Use It → Get Involved (no `## What Changed` opener)
- [ ] Tag(s) exist in `website/blog/tags.yml`
- [ ] Author exists in `website/blog/authors.yml` (added in this PR if new)
- [ ] No Go package paths / internal file layout mentioned
- [ ] `## Get Involved` is 1-2 sentences ending in a link to `https://github.com/cloudposse/atmos/issues`
- [ ] Cast embedded only if a recording demonstrates *this* change (optional otherwise)
- [ ] `atmos lint docs --changed` is clean of `Vale.Avoid` marketing filler
- [ ] `cd website && npm run build` succeeds — required when JSX, imports, or internal links changed.
      For a prose-only edit, vale is sufficient; the build cannot regress from reworded paragraphs.

## Related skills

- **`roadmap` skill** — link the post's slug into the shipped milestone (`changelog: 'your-slug'`) once
  published. This skill doesn't own `roadmap.js` edits; hand off to the `roadmap` skill for that.
- **`pull-request` skill** — owns the semver-label decision tree that determines whether a post is required at
  all; this skill only owns the post itself once one is required.
