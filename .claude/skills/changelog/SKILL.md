---
name: changelog
description: "Blog post authoring for Atmos: MDX template, frontmatter, website/blog/tags.yml and authors.yml rules, problem-first framing, backtick-opening ban, optional cast embeds, and no-Go-internals leakage. Invoke when writing, editing, or reviewing a website/blog/*.mdx changelog post."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Changelog (Blog Post) Authoring

Use this skill whenever you create or edit a post under `website/blog/`. It is the single source of truth
for the template, tags, authors, and style rules — `CLAUDE.md`, the `pull-request` skill, and the `docs`
skill all point here instead of restating these rules. Don't re-duplicate them elsewhere.

## When a post is required

Only PRs labeled `minor` or `major` need one — see the `pull-request` skill's label decision tree. CI enforces
this via `.github/workflows/changelog-check.yml`, which checks for a new `.mdx` file under `website/blog/`.
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
- Never open the body with `## What Changed` — lead with the problem (see Rule 1).

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

- **Violation** — `2026-07-02-atmos-builds-atmos.mdx` opens: "Atmos now builds itself through a first-class
  Atmos command:" — self-referential and feature-first.
- **Violation** — `2026-06-28-list-dependencies.mdx` opens: "The new `atmos list dependencies` command
  renders..." — feature-first (and also a Rule 2 violation, see below).
- **Correct** — `2026-06-29-ci-log-groups.mdx` opens: "A workflow fails in CI. You open the run and you're
  staring at two thousand lines of undifferentiated output..." — pain first, product named later.
- **Correct** — `2026-07-09-vendor-diff-and-update.mdx` opens: "Bumping a vendored component to a newer
  version has always meant guessing."
- **Correct pattern for a hypothetical vendoring feature**, illustrating the same principle: don't write
  "Atmos doesn't support vendoring, so we added it." Instead: "Projects depend on lots of external artifacts.
  Vendoring is a common technique to bring those into the repo so changes to dependencies aren't opaque. It's
  also supportive of immutable infrastructure." — name the general problem/technique, then the fix.

Structure the body `## The Problem` / `## The Fix` / `## How to Use It` / `## Get Involved`.

## Rule 2 — Never open prose with a backtick

Prose (a sentence, paragraph, or the post intro) must start with a word, not an inline code span or fence.
**Bullets may open with a backtick** — this rule is about prose paragraphs only.

- **Violation** — `2025-10-15-introducing-atmos-auth-list.md:39`: "`atmos auth list` solves these
  challenges..."
- **Violation** — `2026-06-27-git-clone-fork-pr-safety-gate.mdx:9`: "`atmos git clone` is Atmos's native
  replacement for..."
- **Violation** — `2026-06-28-list-dependencies.mdx:18`, `2026-06-04-use-version-ref.mdx:12`: same pattern.
- **Fix pattern**: "The `atmos auth list` command solves these challenges..." — lead with a word, then the
  code span.

## Rule 3 — Cast embedding (optional, preferred when a recording exists)

Only a small minority of recent posts embed a cast — it's a nice-to-have, not a requirement, and should never
block a post. When a recorded demo exists (or is worth recording) under `examples/<name>/` or `demo/casts/...`
per the `atmos-asciicast` skill, embed it near the top of the post, after the intro/truncate:

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

- **Violation** — `2025-12-18-function-registry-package.mdx`: title itself is "New pkg/function Package for
  Format-Agnostic Function Registry"; body names `pkg/function/`, `pkg/yaml/`, `pkg/aws/identity/`. A business
  reader doesn't care about Go package paths.
- **Correct** — `2026-06-29-ci-log-groups.mdx` and `2026-06-28-list-dependencies.mdx` describe mechanisms only
  in terms of commands, flags, and observable output — never Go internals.

## Pre-publish checklist

- [ ] Intro opens on the problem, not the feature, and doesn't open with a backtick
- [ ] Body follows Problem → Fix → How to Use It → Get Involved (no `## What Changed` opener)
- [ ] Tag(s) exist in `website/blog/tags.yml`
- [ ] Author exists in `website/blog/authors.yml` (added in this PR if new)
- [ ] No Go package paths / internal file layout mentioned
- [ ] Cast embedded if a relevant recording exists (optional otherwise)
- [ ] `cd website && npm run build` succeeds

## Related skills

- **`roadmap` skill** — link the post's slug into the shipped milestone (`changelog: 'your-slug'`) once
  published. This skill doesn't own `roadmap.js` edits; hand off to the `roadmap` skill for that.
- **`pull-request` skill** — owns the semver-label decision tree that determines whether a post is required at
  all; this skill only owns the post itself once one is required.
