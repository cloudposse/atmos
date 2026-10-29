# Fix: Resume unfinished production releases with explicit confirmation

**Date:** 2026-10-24

## Summary

Add `atmos release cut --resume` and consolidate the local plan/confirm/execute flow into one Starlark script. Go retains release validation, authorization, and GitHub operations.

## Context

Repeating a normal release cut selects the current source and dispatches a new workflow. Recovery should instead follow the original production release while its tag and draft remain reusable. Published immutable releases must remain outside this recovery path.

## Changes

- Resolve the next milestone's reserved tag and validate the original workflow's saved preparation plan against its source and current release eligibility.
- Bind confirmation to the version, source, workflow run, attempt, and preparation artifact; default confirmation to No.
- Watch active runs or request one failed-job rerun, preserving successful jobs and their artifacts. Wait for the new attempt before watching; uncertain responses never trigger an automatic second request.
- Reject published releases, conflicting identities, ambiguous runs, and missing or expired preparation artifacts. Recovery also works before draft creation when preparation saved its plan.
- Document the retained-artifact requirement and the separate handling of post-publication bookkeeping retries.

## Validation

- Full `go test -tags=mage ./magefiles/release/...` passed with the pinned GoReleaser 2.18.3 rehearsal binary.
- Recovery HTTP fixtures cover failed, cancelled, active, and repeated attempts; publication and authorization boundaries; changed confirmation; missing artifacts; and interrupted or uncertain reruns.
- The actual Starlark interpreter exercises fresh/resumed cuts with accepted, declined, and empty confirmation responses.
- All 38 publication action tests passed, including partial asset retry and conflicting asset rejection.
- Repository custom Go lint reports zero new issues; fix document validation and `git diff --check` passed.
- No production release or live workflow was mutated during validation.

## Follow-ups

None. Artifact retention and the published-release boundary are intentional recovery limits documented in the release skill and PRD.
