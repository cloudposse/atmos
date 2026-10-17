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

When native registration is requested or otherwise authorized, use `gh stack link` for existing PRs
in the intended bottom-to-top order. Inspect current remote membership first, since linking changes
the stack. Do not convert an ordinary chain into a native stack merely to inspect it. Verify the
resulting object and ordering on GitHub, not just local branch ancestry. For example, after
substituting the actual repository and PR number:

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
| `gh stack push` | Publish active stack branches using explicit per-branch leases. Updates are not atomic; reconcile partial success. Requires authorization for any history replacement and a compatible publication cadence. |
| `gh stack sync` | Fetch, reconcile, cascade-rebase, and push branches atomically with leases, then synchronize PR/stack state. **Not a read-only probe.** |
| `gh stack merge` | Atomically merge a contiguous bottom portion, through the chosen PR, or the whole stack. Requires merge authorization. |

After an interrupted push/submit/sync, inspect which branches, PRs, bases, and native stack entries actually
changed before resuming. Do not assume an error means nothing was published.

## Choose propagation for the actual stack type

**Native GitHub stacks require linear history between their branches before merging.** When a
lower branch or trunk update breaks that relationship, GitHub prescribes a cascading rebase.
Do not use parent merge commits as a general substitute for this native-stack repair. A clean
local merge, or a repository that permits merge commits, does not establish native merge eligibility.
The method used to land the stack (merge, squash, or rebase) is a separate choice.
See GitHub's [linear-history requirement](https://docs.github.com/en/pull-requests/reference/stacked-pull-requests#linear-history).

Batch confirmed fixes before a necessary rebase, then validate the affected branches once. Deferring
propagation while collecting fixes can save CI and review quota; it does not waive the native
linearity requirement. Inspect the installed `gh stack rebase` scope flags and the actual dependency
boundary instead of automatically rewriting every branch after each finding. `rebase` and `sync`
are mutations, never status commands. Follow the detailed
[native-stack repair procedure](references/maintenance.md#repair-native-stack-history).

Before requesting reviews, verify that every dependent layer in the intended review block contains
its current direct parent and has that parent as its sole merge base. Assess the bottom PR's trunk
freshness separately; an unrelated trunk advance alone does not require rewriting or delaying reviews.
Do not skip stale intermediate layers by
merging a lower update directly into a higher PR. Deferred propagation is unfinished integration;
hold reviews for affected heads until the batch is stable. See the
[parent-chain checks](references/maintenance.md#validate-the-parent-chain-before-requesting-reviews).

For an ordinary dependent PR chain outside native stacks, signed parent merges may be appropriate
when repository policy and the user's history preferences permit them. Verify that classification
first; do not unlink a native stack to avoid its requirements without user authorization.

GitHub documents retained approvals after a native stack rebase when the PR diff is unchanged,
even with stale-approval dismissal enabled. Do not assume every rebase loses every approval, or
that every approval will survive a conflict resolution. Check GitHub review applicability, CodeRabbit
review evidence, and CI separately; a retained review can refer to an earlier commit. Atmos's Quality
Gate currently requires exact-head CodeRabbit approval. Read the live review's commit ID: GitHub may
associate a retained approval with the new head, satisfying that requirement without another review.
Unchanged diffs or patch IDs do not guarantee retention. Before publication, report the affected
approved PRs and contiguous prefix at risk. Verify the gate implementation and use incremental review for a confirmed gap; changed
SHAs never automatically justify full CodeRabbit reviews. See
[approval and CI verification](references/maintenance.md#verify-approvals-after-a-native-rebase).

Publish normal updates with ordinary fast-forward pushes. If a push is rejected, fetch and inspect
the remote changes; do not automatically retry with a force-push flag. An explicit lease is an
exception for a specifically justified, authorized operation, not the default publication command.

Do not merge new trunk commits through every PR merely because trunk advanced. Check native
linearity, conflicts, required freshness, and the combined result first. Explain the affected range
and validation/review cost before a required rewrite. Honor explicit history restrictions and
existing authorization; do not add a new permission round when the repair is already authorized.

## Build approvals from the bottom

Prioritize a contiguous approved bottom block, not an increasing count of approvals scattered
through the stack. Verify applicable reviews in native stack order, including retained approvals
after a native rebase, and distinguish those from the repository's exact-head CodeRabbit gate. Identify
the first gap under the requested approval criterion, and
spend review quota on closing gaps in the intended bottom block before higher layers. Preserve
already approved heads where possible; necessary changes require fresh evidence. Report the
approved bottom prefix separately from total approvals and from the prefix with all merge
requirements satisfied. Verify the actual boundary before proposing a split or partial merge;
see [review scheduling and reporting](references/maintenance.md#build-a-contiguous-approved-bottom-block).

## Keep each PR reviewable

**Stay below the 150-file review boundary.** The repository quality gate currently requires
`changedFiles < 150` (at most **149**); see `qualityFileCount` in
`magefiles/qualitygate/quality.go`. Apply the stricter of the user's budget, repository gate, and bot
configuration, and verify those when they change. This is not a GitHub diff limit. Check it when
planning layers, after moving files or rebasing/updating parents, and again after publication. Count against the PR's actual parent, not `main` for every layer:

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
and coordinate final-head CI/coverage and applicable CodeRabbit approval evidence. Report those states separately.
Default to incremental CodeRabbit requests (`@coderabbitai review`). Unattended queues must not
request full reviews or automatically escalate to them; see the reference for handling missing approvals.

Merge only when explicitly authorized. When available and compatible with repository rules,
`gh stack merge` can merge the selected bottom portion atomically, avoiding a repeated
merge-one/rebase/review cycle. GitHub still enforces protection rules and native stack linearity;
atomic merge is not a bypass. Approval to repair history is also separate from approval to land PRs.
Verify the selected PRs, current heads, and merge method before invoking it. Do not treat approval
to fix CI or request reviews as approval to merge or close PRs.
