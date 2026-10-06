---
name: homebrew
description: "Prepare, verify, and submit a Homebrew/homebrew-core formula PR for atmos (or fix an existing one that was closed for missing the template). Covers the real PR template, the AI/LLM disclosure rules, the 50-character commit-subject limit, and how to actually run brew install --build-from-source / brew test / brew audit --strict / brew style locally without needing a full homebrew-core clone. Invoke before opening a Homebrew formula PR, when a Homebrew PR got auto-closed for missing the template or looking AI-written, or when asked to update the atmos Homebrew formula."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Homebrew Formula PR Workflow (atmos)

Use this skill for any change to `Formula/a/atmos.rb` in `Homebrew/homebrew-core` (a
different repo than `cloudposse/atmos` — all `gh` commands below need
`--repo Homebrew/homebrew-core`). It exists because a previous attempt got
auto-closed by BrewTestBot for looking AI-generated and skipping the real
template — this skill is how to not repeat that.

## Do not open a new PR to fix a closed one

If BrewTestBot closes a PR with the `incomplete-pr-template` message, it says
explicitly: **"Do not open a new pull request for this."** Editing the body of
the existing (closed) PR to complete the template makes the bot reopen it
automatically. Check state first:

```bash
gh pr view <num> --repo Homebrew/homebrew-core --json state,title,body,comments
```

Before submitting anything, also confirm there isn't a duplicate open PR for
the same formula (the template's own checklist requires this):

```bash
gh pr list --repo Homebrew/homebrew-core --search "atmos in:title" --state open --json number,title,url
```

## The real PR template

Fetch it fresh rather than assuming — it changes:

```bash
gh api repos/Homebrew/homebrew-core/contents/.github/PULL_REQUEST_TEMPLATE.md --jq '.content' | openssl base64 -d -A
```

The template body is **just the checklist** — unlike atmos's own PR template,
there are no `## what`/`## why` headers. That does not mean explanation is
optional: real merged homebrew-core PRs consistently open with a free-text
paragraph (often with links to the upstream fix/issue) above the checklist —
e.g. "Fixes #301233 by building libraries statically... Roughly similar to
Fedora's layout, but they also delete the Flang RT headers..." (PR #301251).
Write that explanation; just don't force it into headed sections the
template doesn't ask for. The checklist itself is what BrewTestBot actually
gates on. As of this writing, the checklist covers:

- Followed the [contributing guidelines](https://github.com/Homebrew/homebrew-core/blob/HEAD/CONTRIBUTING.md)?
- Commits follow the [commit style guide](https://docs.brew.sh/Formula-Cookbook#commit)?
- No other open PR for the same formula?
- Built locally with `HOMEBREW_NO_INSTALL_FROM_API=1 brew install --build-from-source <formula>`?
- `brew test <formula>` passing?
- `brew audit --strict <formula>` passing (or `brew audit --new` for a new formula)?
- AI/LLM disclosure checkbox (see below — this is the one that sank the first attempt).

**Never tick a checkbox for something you have not actually run.** The
template's own header says so, and BrewTestBot's heuristics for
"looks AI-written" are tuned to catch exactly that.

## AI/LLM disclosure (read this before writing the PR body)

Full source of truth: `CONTRIBUTING.md`'s "Artificial intelligence" section
and https://docs.brew.sh/Responsible-AI-Usage. Fetch both fresh:

```bash
gh api repos/Homebrew/homebrew-core/contents/CONTRIBUTING.md --jq '.content' | openssl base64 -d -A
```

Key rules, and how the first attempt violated them:

- **Disclosure must be in the initial PR body**, not added after the fact.
  The first attempt disclosed nothing at all, which is a real
  CONTRIBUTING.md violation, not just a checkbox omission.
- **You must review AI output *before* asking anyone to review it.** The
  first attempt's body literally said *"I haven't run a local brew audit...
  happy to iterate"* — an admission of the opposite. Never submit with that
  framing. Do the verification first (see below), then write the PR body
  describing what you actually ran and what it showed.
- **Never attribute a commit to AI** (no `Co-authored-by`/`Assisted-by`
  trailers on the formula-repo commit — this is stricter than atmos's own
  commit convention, which does add a `Co-Authored-By` trailer; do not carry
  that habit into a homebrew-core commit).
- The PR author must personally answer maintainer questions and review
  comments going forward — this is a commitment about the human's future
  conduct. Confirm this with the user explicitly rather than ticking it
  unilaterally on their behalf; see the parent `pull-request` skill's general
  norm of not taking actions that bind the user without their sign-off.
- **Attribute actions to whoever actually did them.** A first draft of the
  disclosure paragraph said "I built... I ran `brew test`..." in the human's
  voice, when the agent ran those commands and the human reviewed the
  results. Write it as: the agent ran the verification, the human reviewed
  the diff and results. Getting this backwards is the same kind of dishonest
  framing that got the original PR closed, just inverted.
- **Keep the disclosure itself short** — one sentence stating the tool, plus
  a short list of what was actually verified. It does not need its own
  essay; the checklist above already covers most of what a reviewer needs.
- **Checking the disclosure box does not auto-close the PR.** Verified
  directly against the policy doc — disclosure is a transparency
  requirement, not disqualifying. The "one AI-assisted PR open at a time"
  clause applies to non-maintainers *of Homebrew core*, which is unrelated
  to being a maintainer of the upstream project the formula packages.

## Commit message style (the second thing the first attempt got wrong)

From `docs.brew.sh/Formula-Cookbook#commit`:

- Subject line **50 characters or less**. Check before committing:
  `echo -n "atmos: your subject" | wc -c`.
- Format: `foobar 7.3 (new formula)` for new formulae, `foobar 7.3` for a
  version bump, `foobar: fix flibble matrix.` for any other fix — formula
  name always leads.
- If the summary can't fit in 50 characters, it's probably two commits.

The first attempt's subject (`atmos: build with GOFIPS140=latest for parity
with upstream releases`, 68 chars) blew past this. Amend before pushing:

```bash
git commit --amend -m "atmos: build with GOFIPS140=latest"
```

## Actually running brew install/test/audit/style locally

`atmos` (and most core formulae) installs via Homebrew's JSON API by default,
so there is no local git clone of `homebrew-core` to edit directly — `brew
formula atmos` only gives you a read-only cached `.rb`. `HOMEBREW_NO_INSTALL_FROM_API=1
brew install --build-from-source atmos` on that cached file fails with "Homebrew
requires formulae to be in a tap". Use a disposable local tap instead of
cloning the full `homebrew-core` tap (large, slow):

```bash
brew tap-new local/atmos-pr-test
tap_dir="$(brew --repository local/atmos-pr-test)"
cp "$(brew formula atmos)" "$tap_dir/Formula/atmos.rb"
# Apply the same edit the real PR makes, e.g.:
#   ENV["GOFIPS140"] = "latest"
$EDITOR "$tap_dir/Formula/atmos.rb"

# `no_autobump!` (and some other directives) are only valid in official taps —
# strip them for local-tap testing only; they stay untouched in the real PR.
perl -pi -e 's/^\s*no_autobump!.*\n//' "$tap_dir/Formula/atmos.rb"

# atmos is likely already installed from homebrew/core; a same-name formula
# from a different tap can't coexist. Uninstall first, restore after.
brew uninstall atmos

HOMEBREW_NO_INSTALL_FROM_API=1 HOMEBREW_NO_AUTO_UPDATE=1 \
  brew install --build-from-source local/atmos-pr-test/atmos
brew test local/atmos-pr-test/atmos
brew audit --strict local/atmos-pr-test/atmos
brew style local/atmos-pr-test/atmos

# Restore the real install and clean up — do not leave the test tap installed.
brew uninstall local/atmos-pr-test/atmos
brew install atmos   # reinstalls the official bottle from homebrew/core
rm -rf "$tap_dir"
```

**Gotcha: false positives from the test-tap scaffolding itself.** Removing
`no_autobump!` (or other local-tap-only edits) can leave stray artifacts —
e.g. a leftover blank line — that `brew audit --strict`/`brew style` will
flag. Before treating a finding as a real problem, `diff` your test copy
against the untouched original and confirm the flagged line isn't just your
own scaffolding:

```bash
tap_dir="$(brew --repository local/atmos-pr-test)"
diff "$(brew formula atmos)" "$tap_dir/Formula/atmos.rb"
```

`brew style` run against a bare file path (not through a tap) produces
unrelated noise (missing Sorbet sigils, missing top-level doc comment, a
false "duplicate method" if the real cached formula is also on disk) — always
run it through the tap name (`local/atmos-pr-test/atmos`), not a raw path.

## Writing the "why" — keep it short

Homebrew's template has no What/Why section, but a short paragraph above the
checklist explaining *why* is normal and helpful. Real merged precedent is
terse: a one-sentence fix (#299644 `bbot: fix build`) and a ~9-line change
with a before/after example and two links (#204084, a single `ENV[...]`
addition — the closest analog to a one-line env-var fix) both merged with no
padding. Match that length, not atmos's own PR-template verbosity. A first
draft of this exact PR ran to four paragraphs restating things already
visible in the diff ("adds no new dependency", "the formula already requires
Go") — cut anything the reviewer can see for themselves in the one-line diff.
Aim for one short paragraph plus one or two links; expand only if the change
is genuinely non-obvious.

Write that paragraph in **ASD-STE100 (Simplified Technical English)** style:
short sentences, active voice, one idea per sentence, plain everyday words —
a Homebrew reviewer skimming a diff, not a blog reader. (This is the opposite
of atmos's own blog-post convention, which explicitly avoids STE100
choppiness — that rule is scoped to blog posts, not this.) Short sentences
and short overall length reinforce each other; don't use STE100's brevity per
sentence as license to add more sentences.

If you are also a maintainer of the upstream project the formula packages
(e.g. Cloud Posse / atmos), say so in one clause and link to a
**pinned-commit permalink** (not a branch) for the source-of-truth line, so
it survives future edits:

```
https://github.com/cloudposse/<repo>/blob/<full-sha>/<path>#L<line>
```

## Updating the PR body

```bash
cat > /tmp/pr-body.md <<'EOF'
...filled-in template...
EOF
gh pr edit <num> --repo Homebrew/homebrew-core --body-file /tmp/pr-body.md
```

Use `--body-file` (see the `pull-request` skill's backtick-escaping gotcha —
it applies here too).

## Related skills

- **`pull-request`** — the analogous workflow for atmos's own PRs onto
  `cloudposse/atmos`; not applicable to homebrew-core (different template,
  different label/changelog rules) but the backtick-escaping gotcha for
  `gh pr create --body` applies equally here.
