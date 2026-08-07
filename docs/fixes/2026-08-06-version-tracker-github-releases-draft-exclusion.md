# Fix: Version Tracker `github-releases` datasource no longer resolves to draft releases

**Date:** 2026-08-06

## Summary

`atmos version track` with `datasource: github-releases` and `desired: latest` (or any range) could resolve
to an unpublished draft GitHub release instead of the actual latest published release, whenever the GitHub
token in use had push/write access to the repo. `pkg/github.GetReleases` now unconditionally excludes draft
releases, fixing the Version Tracker resolver and its sibling callers at the root.

## Context

`gh release view v1.226.0 --repo cloudposse/atmos --json isDraft` confirmed that `atmos version track lock`
had resolved `atmos` to a draft release (`v1.226.0`) instead of the real latest published release
(`v1.225.0`), when run with `datasource: github-releases` and `desired: latest`. Draft releases are only
visible to collaborators via the GitHub API, so this doesn't reproduce with a plain public read-only token —
but `secrets.GITHUB_TOKEN` in a repo's own CI does have that access, which is exactly the situation that
surfaced the bug.

Root cause: `pkg/github/releases.go`'s `GetReleases` lists all releases via `client.Repositories.ListReleases`
(which includes drafts for authorized tokens) and only filtered prereleases (`filterPrereleases`) and by date
(`filterByDate`) — there was no draft filter anywhere. `GetReleases` is shared by three call sites, all
affected: `pkg/version/resolver/github/github.go`'s `releaseCandidates` (the Version Tracker resolver — the
reported bug), `pkg/github/releases.go`'s own `GetReleaseVersions`, and `cmd/version/list.go`'s
`atmos version list`. By contrast, `GetLatestRelease`/`GetLatestReleaseInfo` in the same file use GitHub's
dedicated "get latest release" endpoint, which excludes drafts and prereleases by definition — but the
resolver doesn't use that endpoint; it lists all releases and filters client-side, which is where the gap
was.

Unlike prerelease (a legitimate opt-in choice via `VersionEntry.Prerelease *bool`), there is no legitimate
case for ever resolving to a draft — a draft isn't published and isn't installable/downloadable by
non-collaborators. The fix excludes drafts unconditionally at the shared `GetReleases` layer rather than
adding a new opt-in policy field threaded through `pkg/schema/version.go`, the merge-precedence cascade in
`pkg/version/manager/manager.go`, `resolver.Candidate`, and `pkg/version/resolver/filter.go`.

## Changes

- `pkg/github/releases.go`: added `filterDrafts`, mirroring the existing `filterPrereleases` helper but
  unconditional (uses `release.GetDraft()`, a field already present on `google/go-github/v59`'s
  `RepositoryRelease`). Wired into `GetReleases` immediately after `filterPrereleases`. No changes needed to
  `ReleasesOptions`, `releaseCandidates`, `resolver.Candidate`, or `pkg/version/resolver/filter.go` — every
  caller of `GetReleases` inherits the fix automatically.
- `pkg/github/releases_test.go`: added `TestFilterDrafts`, a table-driven unit test following the existing
  `TestFilterPrereleases` pattern, including a case that mirrors the exact reported scenario (a draft release
  newer than the actual latest published release).
- `docs/prd/atmos-version-management.md`: added a note under the Include/Exclude/Prerelease policy section
  clarifying that draft releases are always excluded from resolution regardless of the `prerelease` setting.

## Validation

- `go build ./pkg/github/... ./pkg/version/... ./cmd/version/...` — clean.
- `go test ./pkg/github/... -run 'TestFilterDrafts|TestFilterPrereleases|TestFilterByDate|TestFetchAllReleases_MockServer' -v` — all pass.
- `go test ./pkg/version/... ./cmd/version/...` — all pass, no regressions.
- `atmos lint --changed` — 0 issues.
- Manual reproduction against live GitHub API not re-run in this session (would require a `GITHUB_TOKEN` with
  push access to `cloudposse/atmos` and depends on current repo release state); the added unit test
  reproduces the reported scenario (draft newer than latest published release) synthetically and confirms
  the fix.

## Follow-ups

None.
