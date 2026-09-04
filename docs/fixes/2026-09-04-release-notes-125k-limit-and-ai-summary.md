# Fix: drafted release body exceeded GitHub's 125,000-character limit

**Date:** 2026-09-04

## Summary

The `release / draft / release` job (`test.yml`'s `release:` job, calling
`cloudposse/.github`'s `shared-go-auto-release.yml` → `shared-auto-release.yml` →
`release-drafter/release-drafter`) started failing on every push to `main`:

```
Validation Failed: {"resource":"Release","code":"custom","field":"body",
"message":"body is too long (maximum is 125000 characters)"}
```

Root cause: `cloudposse/atmos` has no `.github/auto-release.yml` (confirmed via the live GitHub
API and `git ls-tree` against `main` - the path genuinely doesn't exist, unlike every other
CloudPosse repo checked: `bastion`, `build-harness`, `github-commenter`, `test-harness`,
`.github` itself). With no repo config, release-drafter falls back to a `change-template` that
embeds each PR's full body (including CodeRabbit's auto-generated "Summary by CodeRabbit" block)
inside a `<details>` block. The stored draft for `v1.228.0` was 56,410 characters across 23 PRs
(~2.4KB/entry) and kept growing with every merge to `main`, eventually crossing the limit.

A manual edit of the release body via the API (trimmed to 1,991 characters) did **not** fix
reruns: release-drafter recomputes the entire body from scratch off the merged-PR history on
every invocation - it does not diff or append - so the very next run overwrote the trim and
failed identically. Confirmed empirically by editing the release and rerunning the same failed
job: `body is too long` recurred, byte-for-byte the same error.

## Fix

Two independent, durable changes:

1. **`.github/auto-release.yml`** (new, repo-level release-drafter config) +
    **`internal/ci/releasenotes`** (new package) + **`magefiles/ci_release_notes.go`**
    (`release:summarizeNotes`). The org-wide release-drafter template embeds every PR's full body,
    and there is no hook between release-drafter computing that body and writing it, so once it
    passes 125,000 characters the draft cannot be written at all and nothing downstream can run.
    Reading the drafted body back is therefore not a workable source. Instead the repo-level config
    keeps the org's version resolution but reduces `change-template` to a skeleton,
    `- $TITLE @$AUTHOR (#$NUMBER)`, which never approaches the limit, and maps this repo's actual
    labels onto categories: `major` → Breaking Changes, `minor` → Features, `patch` → Bug Fixes (the
    org config filed `patch` under Enhancements and knew no `minor`, so fixes read as enhancements,
    no Bug Fixes chapter ever appeared, and features sat uncategorized at the top with no heading).
    The renderer also gives any still-uncategorized group an "Other Changes" heading when the
    release has categorized groups, and converts the `<summary>` line's Markdown (bot author links,
    backticks) to HTML, since GitHub never renders Markdown inside that tag. After that draft is written,
    `summarize-notes` fetches each PR's description from the pull-request API (which has no such
    cap), condenses it, and rewrites the release in the org template's exact shape - the same
    category headings and the same collapsible `<details><summary>title @author (#N)</summary>`
    block per PR - with the condensed text as each block's body: one to three sentences of plain
    prose saying what changed and, when the description says so, why, naming the concrete
    commands/flags/config keys; Markdown bullets only when a PR covers separate, unrelated topics
    ("Add X, fix Y, fix Z"), one full sentence per topic. The prompt carries one example of each
    shape. (Earlier passes were tuned live on the draft: one sentence per PR was too little to
    justify expanding a block; demanding 3-6 bullets padded small PRs with restatements; one
    bullet per distinct change atomized related details into parallel stub sentences.) The
    blank lines inside each block are load-bearing: a line starting with `<details>` opens a GFM
    HTML block that swallows everything up to the next blank line as raw HTML, so without them the
    Markdown in the summary line (dependabot's author link, backticks) and in the body rendered raw.
    The notes read and expand as before; only the hidden part is short. With `OPENAI_API_KEY` configured the condensing is done
    by the model (default `gpt-5.6-luna`, overridable with `OPENAI_MODEL`; no `temperature` is
    sent, the gpt-5 family rejects any non-default value); without it, each entry gets CodeRabbit's
    own "Summary by CodeRabbit" block when the PR has one, else its description truncated to 1,200
    characters. If even the summarized body would exceed 120,000 characters the release is
    rewritten as the bare skeleton bullets, which are always valid. Any API failure is caught and
    logged, never fails the job: the skeleton is readable release notes on its own.
    `RELEASE_NOTES_DRY_RUN=1` prints the rewritten body to stdout instead of updating the release,
    for previewing the result (or checking the key/model) against any real release without
    touching it. The parser accepts both the skeleton and the org template's `<details>` shape,
    so a draft produced under either config is handled.
2. **`.github/workflows/release.yml`** (new): the `release:` job (and the new summarization job)
    moved out of `test.yml` into their own `workflow_run`-triggered workflow, gated on
    `github.event.workflow_run.conclusion == 'success'`. Previously `release:` lived inside
    `test.yml` with `needs: [test, terraform-registry-cache, kubernetes-e2e, lint, mock, k3s,
    floci, floci-go, docker, validate, container-step, magefiles]` - a release-drafter or
    summarization failure showed up as the "Tests" workflow failing on `main`, conflating two
    unrelated concerns and making this exact incident confusing to triage. The `workflow_run`
    trigger also drops the hand-maintained job-name list: it fires only after the whole Tests
    suite has actually finished and passed.
3. `release.yml`'s `draft` job also picks up the fix from PR #3039 (bumping the
    `shared-go-auto-release.yml` pin to one that declares `id-token: write`/`contents: read` at its
    own top level, plus the caller-side `contents: read` a job-level `permissions:` block requires
    explicitly) - folded in directly rather than left to conflict with #3039's now-superseded edit
    of `test.yml`'s copy of this job.

## Validation

- `go build ./...`, `go vet -tags mage ./...` - clean.
- `go test ./internal/ci/releasenotes/... -cover` - 95.6% coverage, including a round-trip test
  (render → `ParseDraftedBody` → identical entries) that pins the output to release-drafter's shape.
- `atmos lint --changed` (`custom-gcl run --new-from-rev=origin/main`) - 0 issues.
- `atmos ci validate .github/workflows/test.yml .github/workflows/release.yml` - both valid.
- Validated live with a real key against the actual `v1.228.0` draft (release `379499446`, whose
  body is the skeleton: 19 PRs under 2 category headings):
  `RELEASE_NOTES_DRY_RUN=1 go tool mage release:summarizeNotes cloudposse/atmos 379499446` fetched
  19 PR descriptions, made one model call, and produced 19 `<details>` blocks under the 2 headings
  in 4,942 characters; written to the draft itself afterwards and checked in the GitHub UI. Earlier live attempts (against `v1.228.0-rc.2`, an org-template draft) had
  surfaced two things fixed along the way: `temperature: 0.3` returned `400 unsupported_value` on
  the gpt-5 family, and `gpt-5.6-luna` returned `403 model_not_found` until the cloudposse OpenAI
  project was granted the 5.6 models, after which the call flapped between 403 and success for
  several minutes while the grant propagated - a project setting, not a code problem, and the job's
  log-and-skip behavior covers the propagation window.
- Not yet validated in CI: the `summarize-notes` job needs `OPENAI_API_KEY` configured as a repo/org
  secret before it does anything beyond log-and-skip; the `draft`/summarization split needs a real
  push-triggered `Tests` run to complete before `workflow_run` fires it.

## Follow-ups

- `OPENAI_API_KEY` secret needs to be added (repo or org level) for model summaries; without it the
  job still runs with CodeRabbit/truncated fallbacks.
- Release-drafter reads `.github/auto-release.yml` from the default branch, so the skeleton template
  takes effect on the first `Tests` run on `main` after this merges; that run also repairs the
  current oversized draft.
- PR #3039 (the standalone `shared-go-auto-release.yml` pin bump on `test.yml`) is superseded by
  this change once merged, since the same fix now lives in `release.yml` instead.
