---
name: gh-stack
description: "Create, link, repair, and maintain dependent GitHub PRs with gh stack. Use before creating or updating stacked branches, propagating a lower-layer fix, resolving stack conflicts, or coordinating stack CI and CodeRabbit reviews. Covers native stack registration, preserving reviews, the 150-file review limit, and publication rate limits. Not Atmos configuration stacks."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "2.0.0"
---

# GitHub PR stacks

Use this skill for dependent pull requests. Read
[maintenance and recovery](references/maintenance.md) before changing an existing published stack
or operating its CI/review queue. Use the [pull-request skill](../pull-request/SKILL.md) for labels,
signing, descriptions, and release documentation.

## Discover the tool and the actual stack

Start with `gh stack --help` and the relevant subcommand's `--help`; commands and supported flags
can change. If the extension is missing, identify `github/gh-stack` as the required extension
(`gh extension install github/gh-stack`). Follow the environment's installation permissions;
do not silently replace native stacks with an unregistered branch chain.

A feature branch based on another feature branch is only a dependency chain. GitHub's native
stack is a separate object. Creating PRs with `gh pr create --base <parent>` alone does not register
that object. Check both the configured base branches and native stack membership. A bottom PR may
target `main` and still belong to a stack; also look for open PRs based on its head branch.

For existing PRs, use `gh stack link` with the intended bottom-to-top order. Inspect current remote
membership first, since linking changes the stack. Verify the resulting object and ordering on GitHub,
not just local branch ancestry. For example, after substituting the actual repository and PR number:

```bash
gh stack link <bottom-pr> <middle-pr> <top-pr>
gh api graphql -f query='{ repository(owner:"OWNER", name:"REPO") {
  pullRequest(number:123) { stack { number size entries(first:100) {
    nodes { position pullRequest { number } }
  } } }
} }'
```

Paginate if the returned entries do not cover the reported size. `gh stack link` can register remote
PRs without initializing local tracking; a local `gh stack view` failure alone does not prove the
remote stack is absent. Inspect the installed checkout/link help to reconcile local tracking.

If the user requests an independent PR, branch from its requested trunk, explicitly target that
trunk, and do not link it into the active stack or import the stack's implementation commits.

## Commands and side effects

The installed command help is authoritative. Current command roles are:

| Command | Purpose and publication effects |
|---|---|
| `gh stack init` / `add <branch>` | Initialize local stack tracking / create the next branch. |
| `gh stack view` | Inspect the locally tracked stack. |
| `gh stack checkout <identifier>` | Resolve a stack, PR, URL, or branch and check it out; may fetch remote tracking. |
| `gh stack switch` | Interactive branch picker; use `checkout` for an explicit identifier. |
| `gh stack link` | Register/update native membership. Branch-name arguments also push branches and create missing PRs; use PR numbers/URLs when only linking existing PRs. |
| `gh stack submit` | Push branches, create/update PRs and bases, then register/update the native stack. These phases are not one atomic transaction. |
| `gh stack rebase` | Cascade a local rebase onto updated parents; does not publish the result. |
| `gh stack sync` | Fetch, reconcile, cascade-rebase, and push branches atomically with leases, then synchronize PR/stack state. **Not a read-only probe.** |
| `gh stack merge` | Atomically merge a contiguous bottom portion, through the chosen PR, or the whole stack. Requires merge authorization. |

After an interrupted submit/sync, inspect which branches, PRs, bases, and native stack entries actually
changed before resuming. Do not assume an error means nothing was published.

## Preserve reviews by choosing when to propagate

A changed lower PR does **not** automatically require rebasing every descendant. First compare its
new head with each affected child's actual published head and base. If the child remains mergeable,
its intended diff is intact and within the file limit, and no required validation or dependency needs
the new parent in its history yet, defer propagation and batch remaining fixes.

For published branches, prefer signed merges of the updated parent into affected children when
propagation is needed and repository policy allows merge commits. This preserves existing commit
history; it does **not** guarantee that GitHub will retain approvals. Any new head may require fresh
CI and review. Rebase only when the user chooses it or a verified repository requirement needs it;
`gh stack rebase`/`sync` are not routine status or cleanup commands.

Do not merge new trunk commits through every PR merely because trunk advanced. Check actual
conflicts, required freshness, and the combined merge result first. When an update is necessary,
explain its validation/review cost and batch it with already confirmed fixes. Existing authorization
still applies; do not add a new approval round for ordinary authorized repairs.

## Keep each PR reviewable

**Stay below the 150-file review boundary.** The repository quality gate currently requires
`changedFiles < 150` (at most **149**); see `qualityFileCount` in
`magefiles/qualitygate/quality.go`. Apply the stricter of the user's budget, repository gate, and bot
configuration, and verify those when they change. This is not a GitHub diff limit. Check it when
planning layers, after moving files or merging parents, and again after publication. Count against the PR's actual parent, not `main` for every layer:

```bash
git merge-base --all <actual-parent-sha> <candidate-head-sha>
git diff --name-only <actual-parent-sha>...<candidate-head-sha>
gh pr view <number> --json changedFiles,headRefOid,baseRefName,baseRefOid
```

Count the complete path list (use NUL-delimited output in automation); do not rely on a truncated
API file listing or count only new edits. Multiple merge bases make the comparison ambiguous:
repair the ancestry or investigate before accepting its count. Keep some room below the effective limit when
splitting work. Prefer moving cohesive docs, examples, casts, or tests with their owning behavior
to an appropriate existing subsequent PR when authorized; recheck both layers' independent validity,
counts, and native stack membership. Do not create an extra PR contrary to the user's requested shape.

## Worktree and ownership safeguards

- Use clean, isolated worktrees for independent layer fixes. Give parallel agents ownership of
  specific layers/files and keep one coordinator responsible for propagation, publication, and
  review requests. Do not let multiple workers push the same branch or spend the same review quota.
- Checkout does not clear staged or unstaged edits to files shared between branches. Before switching,
  inspect `git status --short`, `git log -3 --oneline`, and `git diff --cached --stat`. Preserve unfinished
  work; do not discard or stash it to get a clean checkout. Unstaging alone does not isolate edits.
- An affected-file hook can inspect inherited committed changes, while missing a staged-only path.
  Diagnose the owning layer before fixing a hook failure; a passing hook alone is not proof that all
  staged files were checked. Run applicable checks explicitly in isolated worktrees when needed.
- Fix confirmed findings at the earliest affected layer. During propagation, preserve later intentional
  behavior as well as the fix: later docs may correctly supersede an earlier rollout phase's wording.
  Resolve overlapping logic semantically; neither concatenating both sides nor taking one side wholesale
  is a general conflict-resolution strategy.
- Preserve signed commits and hooks. If signing fails (including a locked signing agent), retain the
  prepared work and report the blocker; do not bypass signing to publish.

## Finish the requested scope

Use the [maintenance reference](references/maintenance.md) to freeze candidate heads, publish safely,
and coordinate exact-head CI, coverage, and CodeRabbit evidence. Report those states separately.

Merge only when explicitly authorized. When available and compatible with repository rules,
`gh stack merge` can merge the selected bottom portion atomically, avoiding a repeated
merge-one/rebase/review cycle. GitHub still enforces protection rules; atomic merge is not a bypass.
Verify the selected PRs, current heads, and merge method before invoking it. Do not treat approval
to fix CI or request reviews as approval to merge or close PRs.
