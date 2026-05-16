---
name: pull-request
description: "PR workflow: pick the right semver label (no-release / patch / minor / major), decide when to add a changelog blog post, when to update the roadmap, and how to do each correctly. Invoke before opening a PR or when touching an existing PR's release docs."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Pull Request Preparation (Atmos)

Use this skill **every time you open or update a PR**. It encodes three policies that the Atmos repo enforces via CI and that the team has been burned by repeatedly:

1. **Every PR needs exactly one semver label.** Unlabeled PRs fail the `PR Semver Labels` CI check.
2. **`minor` and `major` PRs require a blog post AND a roadmap update.** The `Check for changelog and roadmap updates` workflow gates merging on both.
3. **`featured[]` in the roadmap is curated.** Never auto-promote a shipped milestone into `featured[]` — only the user decides.

If you violate any of these, CI fails and the PR can't merge. Follow this skill end-to-end before pushing, not after.

## The label decision tree

The Atmos repo has four mutually exclusive release labels (plus other category labels that don't affect release docs):

| Label        | When                                                                                                  | CI requires blog + roadmap? |
| ------------ | ----------------------------------------------------------------------------------------------------- | --------------------------- |
| `no-release` | Internal refactor, new internal helper, dependency bump with zero user-visible behavior, docs fixes   | No                          |
| `patch`      | Bug fix, performance fix, error-message improvement, small UX polish — user-visible but not a feature | No                          |
| `minor`      | New user-visible feature, new flag, new command, new config option, default flip that users see       | **YES**                     |
| `major`      | Breaking change: removed/renamed flag, schema migration, CLI behavior reversal                        | **YES**                     |

### How to choose

Ask, in order:

1. **Would a user upgrading Atmos see ANY visible change?** Behavior, output, errors, performance, new flags, new commands, removed flags, changed defaults? If no, use `no-release`. This is the most common case for foundation/plumbing PRs.
2. **Is this a breaking change?** Removed flags, renamed flags, changed semantics, schema bumps that require user action? Use `major`. Document the migration in the blog post.
3. **Is this net-new functionality?** New command, new flag, new config option that users will adopt? Use `minor`. Write a blog post.
4. **Anything else user-visible** (bug fix, UX polish, error-message rewrite, output formatting): use `patch`. No blog post required.

**Common mistake:** labeling a plumbing PR as `minor` because it's part of a larger feature. The label is per-PR, not per-feature. If a five-PR stack adds one user-visible feature, only the **final** PR that wires it up gets `minor` — the four foundation PRs are `no-release`.

**Another common mistake:** flipping a default value and labeling `patch` because "it's just a default." If users see different behavior on upgrade, that's `minor` (or `major` if it could break their workflow).

### Apply the label as the first action

After opening the PR, immediately:

```bash
gh pr edit <pr-number> --add-label <label>
```

Don't wait for CI to complain — you already know the label.

If you're opening multiple related PRs that are foundation work, label them all `no-release` up front:

```bash
for pr in 2417 2418 2419; do gh pr edit "$pr" --add-label no-release; done
```

**Changing a label later requires removing the old one first.** `--add-label` doesn't replace — running `--add-label minor` on a PR already labeled `patch` leaves *both* attached, which violates the "exactly one semver label" invariant and fails the `PR Semver Labels` CI gate:

```bash
# Wrong — leaves both labels:
gh pr edit <pr-number> --add-label minor

# Right — remove the old semver label first, then add the new one:
gh pr edit <pr-number> --remove-label patch --add-label minor
```

`gh pr edit` accepts `--remove-label` and `--add-label` in the same invocation, so the relabel is atomic.

## Blog post (changelog) rules

A blog post is **only** required when the PR is labeled `minor` or `major`. CI checks for a new `.mdx` file under `website/blog/`. If you have the wrong label, fix the label rather than writing a blog post you don't need.

### When you DO write a blog post

- File path: `website/blog/YYYY-MM-DD-feature-name.mdx` (kebab-case slug after the date).
- Use `.mdx` (not `.md`). MDX lets you embed components like `<EmbedFile>` and `<Screengrab>`.
- Front matter is YAML with fields: `slug`, `title`, `authors`, `tags`.
- Body opens with a 1–2 sentence intro, then `<!--truncate-->`, then the long-form content under section headers.

Template:

```mdx
---
slug: descriptive-slug
title: "Clear, concise title"
authors: [your-github-handle]
tags: [feature]
---

One-paragraph intro that tells the reader why this matters.

<!--truncate-->

## What changed

...

## Why it matters

...

## How to use it

...

## Get involved

...
```

### Tags (MUST read before picking)

Only use tags defined in `website/blog/tags.yml`. **Never invent a tag** — CI doesn't check, but the website will render an empty tag page.

User-facing tags (use one of these for user-visible PRs):
- `feature` — new capability
- `enhancement` — improvement to an existing feature
- `bugfix` — bug fix
- `dx` — developer-experience improvement
- `breaking-change` — required user action
- `security` — security-related
- `documentation` — docs-only changes (also pair with `no-release`)
- `deprecation` — feature scheduled for removal

Internal tag (rarely the right pick):
- `core` — contributor-only changes with zero user impact (but if user impact is zero, you should be `no-release` anyway — `core` is for the rare case where you want a changelog entry for contributor visibility)

### Authors

Use an existing author from `website/blog/authors.yml`. If the committer isn't there, **add an entry to `authors.yml` in the same PR**.

### Internal refactors do NOT get blog posts

Per [`.claude/agents/roadmap.md`](../../agents/roadmap.md): if a user upgrading Atmos would see no change in behavior, output, errors, performance, or available commands/flags, **do not write a changelog post**. Refactors are visible in PR descriptions and `git log`; that is sufficient.

Engineering wins like "complexity 247→10" or "test coverage 60%→95%" can live as milestones inside the `quality` initiative on the roadmap — but **without** a `changelog:` field and **without** a `website/blog/*.mdx`.

## Roadmap rules

A roadmap update is **only** required when the PR is labeled `minor` or `major`. CI checks for changes to `website/src/data/roadmap.js`.

**Delegate the actual edit to the `roadmap` agent** — it knows the data shape, the no-auto-feature rule, and how to compute progress percentages. The shape it follows:

- A new shipped milestone goes under the relevant initiative's `milestones[]` array, with:
  - `status: 'shipped'`
  - `quarter: 'qN-YYYY'`
  - `pr: <pr-number>` (the GitHub PR number)
  - `changelog: '<blog-post-slug>'` (matches the blog post `slug`)
  - `description` — what shipped
  - `benefits` — why users care

Example milestone entry:

```js
{
  label: 'Toolchain support for X',
  status: 'shipped',
  quarter: 'q2-2026',
  pr: 2416,
  changelog: 'toolchain-x-support',
  description: '...',
  benefits: '...',
},
```

After adding the milestone, recompute the initiative's `progress` percentage: `(shipped milestones / total milestones) * 100`.

### NEVER touch `featured[]` unless explicitly asked

The `featured: []` array in `roadmap.js` is a **manually curated** highlight reel — **max 6 strategic initiatives**, not a list of every shipped milestone. The user is the only person who decides what gets featured.

When you ship a milestone:
- ✅ Add it to `initiatives[].milestones[]` with `status: 'shipped'`
- ❌ Do NOT add it to `featured[]`
- ❌ Do NOT reorder existing `featured[]` entries

If the user explicitly asks ("promote X to featured", "add Y to the featured section"), then act — and only then. If you're unsure, ask.

### When NOT to touch the roadmap

If your PR is `no-release` or `patch`, **don't touch the roadmap**. It's not required by CI for those labels, and adding noise milestones dilutes the roadmap's signal value.

## Signed commits (MANDATORY — branch protection blocks merge)

**Every commit on the PR must be GPG- or SSH-signed.** Branch protection on `main` rejects merges of any PR containing unsigned commits — there is no override. If you push unsigned commits, you will rewrite history later to re-sign them, which is painful for reviewers (force-push invalidates their in-progress reviews).

Get this right the first time:

1. Verify your local git is configured to sign automatically before your first commit:

   ```bash
   git config --get commit.gpgsign       # should print: true
   git config --get gpg.format            # "openpgp" or "ssh"
   git config --get user.signingkey       # your signing key
   ```

   If `commit.gpgsign` is not `true`, set it for the repo:

   ```bash
   git config commit.gpgsign true
   ```

2. After your first commit, verify it's signed:

   ```bash
   git log --show-signature -1
   ```

   Look for `gpg: Good signature from ...` or `Good "git" signature for ...`. If you see "no signature found", **stop and fix your git config before pushing**.

3. **Never bypass signing with `--no-gpg-sign` or `-c commit.gpgsign=false`** even temporarily. The CLAUDE.md rules forbid this, and the resulting commit cannot be merged.

If you discover unsigned commits already on the branch:

```bash
# For the last N commits (interactive rebase, sign each):
git rebase --exec 'git commit --amend --no-edit -S' -i HEAD~N
git push --force-with-lease
```

Always use `--force-with-lease` (not `--force`) to avoid clobbering work from other sessions on the same branch.

## Pre-push checklist

Run through these in order before `git push`. Every item has burned someone before.

1. **Identify the smallest accurate label.** Use the decision tree above. Default to `no-release` for plumbing.
2. **Confirm commit signing is on** (see above). One unsigned commit blocks the entire PR from merging.
3. **If `minor` or `major`:**
   - Create a blog post at `website/blog/YYYY-MM-DD-<slug>.mdx`.
   - Read `website/blog/tags.yml` and pick a defined tag.
   - Check `website/blog/authors.yml` for your handle; add yourself if missing.
   - Delegate the roadmap update to the `roadmap` agent (do not touch `featured[]`).
4. **Build the website** to verify MDX renders: `cd website && npm run build`.
5. **Commit and push.**

## Post-push / PR checklist

Once the branch is on GitHub, finish the workflow:

1. **Open the PR** with title (under 70 chars) + body using the project's PR template (what / why / references). See the gotcha below about `gh pr create` body formatting.
2. **Apply the label immediately** after opening: `gh pr edit <num> --add-label <label>`.
3. **If the PR fixes a tracked issue:** include `Closes #<issue>` in the PR body so it auto-closes on merge.
4. **Check CI status** after the first push: `gh pr checks <num>`. If `PR Semver Labels`, `Check for changelog and roadmap updates`, or signed-commit verification fails, fix it before requesting review.

## Gotcha: `gh pr create --body` and backtick escaping

Do NOT escape backticks (`` \` ``) inside a single-quoted heredoc passed to `gh pr create --body`. The single-quoted form preserves the backslashes literally, so GitHub renders them as escaped characters instead of rendering the code spans. The result is a PR body full of `\`atmos.yaml\`` instead of `atmos.yaml`.

**Wrong** (produces visible `\`` in the PR body):

```bash
gh pr create --body "$(cat <<'EOF'
This file is named \`atmos.yaml\`.
EOF
)"
```

**Right** — write a `.md` file and pass it with `--body-file`:

```bash
cat > /tmp/pr-body.md <<'EOF'
This file is named `atmos.yaml`.
EOF
gh pr create --body-file /tmp/pr-body.md
```

(Alternatively, the single-quoted heredoc already disables shell expansion, so plain unescaped backticks work — but `--body-file` is more robust because file content survives shell quoting unchanged.)

## Updating an existing PR

If a PR has already been opened without a label, or with the wrong one:

1. Run `gh pr view <num> --json labels` to see what's already attached. Look for any of `no-release` / `patch` / `minor` / `major` — at most one should remain.
2. Apply the decision tree to pick the correct label.
3. **If the PR has no semver label yet:** add it.

   ```bash
   gh pr edit <num> --add-label <label>
   ```

4. **If the PR has a different semver label already:** remove the old one and add the new one in a single invocation, so you never have two semver labels at once (which fails CI):

   ```bash
   gh pr edit <num> --remove-label <old-label> --add-label <new-label>
   ```

5. If you changed from `no-release`/`patch` to `minor`/`major`, you now owe a blog post and roadmap update — add them in a new commit on the same branch.
6. If you changed from `minor`/`major` to a lower label, you can (optionally) remove the blog post and roadmap update in a new commit, but it's also fine to leave them.

## Reference

- CI workflow: `.github/workflows/changelog-check.yml` (release docs gate)
- CI workflow: `.github/workflows/feature-release.yml` (semver label gate)
- Tags: `website/blog/tags.yml`
- Authors: `website/blog/authors.yml`
- Roadmap data: `website/src/data/roadmap.js`
- Roadmap agent: `.claude/agents/roadmap.md`
- Project rules: `CLAUDE.md` (search for "Pull Requests", "Blog Posts", "Roadmap Updates")
