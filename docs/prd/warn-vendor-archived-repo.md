# PRD: Warn When Vendoring from an Archived GitHub Repository

**Status:** Implemented
**Version:** 1.0
**Last Updated:** 2026-03-12
**Author:** nitrocode

---

## Problem Statement

GitHub-archived repositories receive no new commits, issue responses, or security patches. When a user runs `atmos vendor pull` against an archived repo, there is no signal that the dependency is frozen. Engineers typically discover archived dependencies late in security scans or compliance reviews, when remediation is costly.

---

## Solution

Before downloading a remote vendor source, Atmos parses the URI to check whether it targets a GitHub repository. If it does, Atmos calls the GitHub REST API to read the `archived` field. If the repository is archived, Atmos logs a warning and continues:

```
WARN GitHub repository is archived and no longer actively maintained.
     Vendoring from an archived repository may include outdated or unsupported code.
     repository=cloudposse/terraform-null-label component=null-label
```

The check is **best-effort and non-blocking**. Any API failure (network error, rate limit, missing token, 404) is silently ignored and vendoring proceeds normally.

---

## Architecture

### Components

#### `pkg/github/repo.go`

Two new exported functions:

**`ParseGitHubOwnerRepo(uri string) (owner, repo string, ok bool)`**

Extracts the GitHub owner and repository name from a vendor source URI. Returns `ok=false` for non-GitHub sources (OCI, S3, local paths, GitLab, Bitbucket, GitHub Enterprise Server, etc.), so the caller skips the API check.

Supports all vendor URI formats used in practice:

| Format | Example |
|--------|---------|
| Plain go-getter | `github.com/org/repo//path?ref=v1` |
| HTTPS | `https://github.com/org/repo.git//path?ref=v1` |
| go-getter force prefix | `git::https://github.com/org/repo` |
| SCP-style SSH | `git@github.com:org/repo.git//path` |
| SSH scheme | `ssh://git@github.com/org/repo.git//.?ref=v1` |
| git:: + SSH scheme | `git::ssh://git@github.com/org/repo.git//.?ref=v1` |

**Known limitation:** Only `github.com` is checked. GitHub Enterprise Server (GHES) hosts are treated as non-GitHub sources and silently skipped.

**`IsRepoArchived(owner, repo string) (bool, error)`**

Calls `GET /repos/{owner}/{repo}` via the GitHub REST API and returns the value of the `archived` field. Returns an error on any failure (network, 404, rate limit). Callers should ignore the error and continue.

Uses `newGitHubClient(ctx)` from the existing `pkg/github` package, which honors `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN` for authenticated requests.

#### `internal/exec/vendor_github_archive.go`

**`warnIfArchivedGitHubRepo(uri, component string)`**

Parses the URI, calls `IsRepoArchived`, and logs a warning if the repository is archived. Any error from the API call is discarded so vendoring is never blocked.

#### `internal/exec/vendor_component_utils.go`

Calls `warnIfArchivedGitHubRepo` for `pkgTypeRemote` sources in `ExecuteComponentVendorInternal`, before the download begins.

#### `internal/exec/vendor_utils.go`

Calls `warnIfArchivedGitHubRepo` for `pkgTypeRemote` sources in `processAtmosVendorSource`, before processing vendor manifest targets.

### Decision Flow

```
vendor pull triggered
        |
        v
determinePackageType()
        |
   pkgTypeRemote? --No--> skip check
        |
       Yes
        |
        v
ParseGitHubOwnerRepo(uri)
        |
    ok=false? -----------> skip check (non-GitHub source)
        |
      ok=true
        |
        v
IsRepoArchived(owner, repo)
        |
     error? -------------> silently ignore, proceed
        |
       false ------------> no warning, proceed
        |
       true
        |
        v
   log.Warn(...)
        |
        v
  vendoring proceeds
```

### Rate Limits

The GitHub API rate limit is 60 requests/hour for unauthenticated callers and 5,000 requests/hour with a token. In CI without a token, rate-limited requests return an error that is silently ignored. Set `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN` to avoid hitting the unauthenticated limit on large vendor manifests.

---

## Files Modified

| File | Change |
|------|--------|
| `pkg/github/repo.go` | New: `ParseGitHubOwnerRepo` and `IsRepoArchived` |
| `pkg/github/repo_test.go` | New: 22 unit tests for `ParseGitHubOwnerRepo` |
| `internal/exec/vendor_github_archive.go` | New: `warnIfArchivedGitHubRepo` helper |
| `internal/exec/vendor_component_utils.go` | Call archive check for remote component sources |
| `internal/exec/vendor_utils.go` | Call archive check for remote vendor manifest sources |
| `website/blog/2026-03-12-warn-vendor-archived-repo.mdx` | Blog post |
| `website/src/data/roadmap.js` | Roadmap milestone |

---

## Testing Strategy

### Unit Tests (`pkg/github/repo_test.go`)

22 table-driven test cases for `ParseGitHubOwnerRepo`:

- Plain go-getter URIs (`github.com/org/repo`)
- URIs with subdirectory and query params
- HTTPS URLs with and without `.git` suffix
- `git::` force-prefix variants
- `ssh://` URLs with `.git` suffix and subdirectory delimiter
- SCP-style (`git@github.com:org/repo.git`) with and without subdirectory
- Negative cases: GitLab, Bitbucket, S3, OCI, local path, empty string

### Integration Tests

No dedicated integration tests exist for this feature. The existing golden snapshot tests pass because the API call fails in CI (no real token), the error is silently ignored, and no output changes.

**Gap:** There are no tests that verify the warning is actually emitted when a repo is archived. A future improvement would mock `IsRepoArchived` to return `true` and assert the warning text appears.

---

## Alternatives Considered

### Block vendoring on archived repositories

Treating an archived source as a fatal error would require explicit action before vendoring can proceed. This breaks teams that intentionally vendor from archived but stable modules. Rejected in favor of a non-blocking warning.

### Cache archive status between runs

Persisting the result in workdir metadata would reduce API calls on re-vendor. This adds state management complexity and would suppress the warning if a previously-active repo becomes archived between runs. Rejected; re-checking on every `vendor pull` is simpler and consistent.

### Require explicit opt-in

A flag or config key would let teams silence the check entirely. The warning fires only when a repo is actually archived, so noise is low. On-by-default is preferable for teams that may not know to enable it. An opt-out mechanism could be added later if needed.

---

## References

- GitHub Issue: #2046
- GitHub Pull Request: #2175
- Blog post: `website/blog/2026-03-12-warn-vendor-archived-repo.mdx`
- GitHub REST API (Get a repository): https://docs.github.com/en/rest/repos/repos#get-a-repository
- Existing GitHub client: `pkg/github/client.go`
- Vendor URI normalization PRD: `docs/prd/vendor-uri-normalization.md`
