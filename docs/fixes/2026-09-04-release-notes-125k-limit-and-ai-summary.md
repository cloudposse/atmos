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

1. **`internal/ci/releasenotes`** (new package) + **`magefiles/ci_release_notes.go`**
    (`release:summarizeNotes`): after release-drafter produces its (still full-body) draft, an
    opt-in step parses its `<details>` entries, sends each PR's title+body to an OpenAI model in
    one batched request, and rewrites the release in release-drafter's exact shape - the same
    category headings and the same collapsible `<details><summary>title @author (#N)</summary>`
    block per PR - with the AI-condensed summary as each block's body instead of the full embedded
    PR description. The notes read and expand the same; only the hidden part got shorter. Entirely
    optional - with no `OPENAI_API_KEY` secret configured it logs why and exits 0, leaving
    release-drafter's own body untouched. Any OpenAI/GitHub API failure is caught and logged, never
    fails the job - this must never be able to block the release path over a summarization hiccup.
    `RELEASE_NOTES_DRY_RUN=1` prints the summarized body to stdout instead of updating the release,
    for previewing the result (or checking the key/model) against any real release without touching
    it. The model defaults to `gpt-5.6-luna` and is overridable with `OPENAI_MODEL`; the request
    sends no `temperature`, because the gpt-5 family rejects any non-default value.
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
- Validated live with a real key (`RELEASE_NOTES_DRY_RUN=1 go tool mage release:summarizeNotes
  cloudposse/atmos 378837775`, the `v1.228.0-rc.2` release with three real entries): body went from
  9,763 to 1,072 characters with every `<details>` block, summary line and category heading intact.
  The first live attempts surfaced two things. `temperature: 0.3` returned `400 unsupported_value`
  on the gpt-5 family - a real defect, fixed by not sending one. And `gpt-5.6-luna` returned
  `403 model_not_found` for the cloudposse OpenAI project, which turned out to be a project
  permission (the project had not been granted the 5.6 models); after the grant, the same call
  flapped between 403 and success for several minutes while it propagated. So a 403 here is a
  project setting to fix, not a reason to change the default, and the job's log-and-skip behavior
  already handles the propagation window.
- Not yet validated in CI: the `summarize-notes` job needs `OPENAI_API_KEY` configured as a repo/org
  secret before it does anything beyond log-and-skip; the `draft`/summarization split needs a real
  push-triggered `Tests` run to complete before `workflow_run` fires it.

## Follow-ups

- `OPENAI_API_KEY` secret needs to be added (repo or org level) to activate summarization.
- PR #3039 (the standalone `shared-go-auto-release.yml` pin bump on `test.yml`) is superseded by
  this change once merged, since the same fix now lives in `release.yml` instead.
