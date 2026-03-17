# PRD: Warn When Vendoring from an Archived GitHub Repository

**Status:** Implemented
**Version:** 1.0
**Last Updated:** 2026-03-12
**Author:** Erik Osterman

---

## Executive Summary

When `atmos vendor pull` fetches a component or stack source from GitHub, Atmos now calls the GitHub Repositories API to check whether the source repository is archived. If it is, Atmos emits a non-blocking warning to alert engineers that the dependency may be stale or unsupported.

**Key Principle:** The check is best-effort and never blocks vendoring. If the GitHub API is unavailable for any reason (network failure, rate limit, missing token), the check is silently skipped and vendoring proceeds normally.

---

## Problem Statement

GitHub-archived repositories are frozen: no new commits, no issue responses, no security patches. When an engineer vendors from an archived repo, they silently introduce a dependency that:

1. **Will never receive security patches** — Any vulnerability discovered after archival remains permanently unfixed in the upstream source.
2. **May be incompatible with newer dependencies** — Archived modules can drift out of compatibility with provider versions or other modules.
3. **Has no upstream support** — Issues and pull requests are disabled; problems cannot be reported or fixed upstream.

Without this warning, there is no signal during the vendor operation that the repository is in a frozen state. Engineers typically discover archived dependencies late — during security scans or compliance reviews — when remediation is expensive and disruptive.

---

## Solution

Before downloading a vendor source, Atmos parses the URI to determine if it references a GitHub repository. If it does, Atmos calls the GitHub REST API to check the `archived` field. If the repository is archived, a warning is logged:

```
WARN GitHub repository is archived and no longer actively maintained.
     Vendoring from an archived repository may include outdated or unsupported code.
     repository=cloudposse/terraform-null-label component=null-label
```

The warning is **informational only** — vendoring always proceeds regardless of the check result.

---

## Architecture

### Components

#### `pkg/github/repo.go`

Two new exported functions:

**`ParseGitHubOwnerRepo(uri string) (owner, repo string, ok bool)`**

Extracts the GitHub repository owner and name from a vendor source URI. Returns `ok=false` for non-GitHub sources (OCI, S3, local paths, GitLab, Bitbucket, etc.), so the caller can skip the API check entirely.

Supports all vendor URI formats used in practice:

| Format | Example |
|--------|---------|
| Plain go-getter | `github.com/org/repo//path?ref=v1` |
| HTTPS | `https://github.com/org/repo.git//path?ref=v1` |
| go-getter force prefix | `git::https://github.com/org/repo` |
| SCP-style SSH | `git@github.com:org/repo.git//path` |
| SSH scheme | `ssh://git@github.com/org/repo.git//.?ref=v1` |
| git:: + SSH scheme | `git::ssh://git@github.com/org/repo.git//.?ref=v1` |

**`IsRepoArchived(owner, repo string) (bool, error)`**

Calls `GET /repos/{owner}/{repo}` via the GitHub REST API and returns the value of the `archived` field. Returns an error when the API call fails; callers that treat the check as best-effort should silently ignore the error.

Uses `newGitHubClient(ctx)` from the existing `pkg/github` package, which honours `ATMOS_GITHUB_TOKEN` / `GITHUB_TOKEN` for authenticated requests and higher rate limits.

#### `internal/exec/vendor_github_archive.go`

**`warnIfArchivedGitHubRepo(uri, component string)`**

Orchestrates the check: parse the URI, call `IsRepoArchived`, and emit a warning via `log.Warn` if the repository is archived. Any API failure is silently swallowed so that vendoring is never blocked.

#### `internal/exec/vendor_component_utils.go`

Calls `warnIfArchivedGitHubRepo` for remote package types (`pkgTypeRemote`) in `ExecuteComponentVendorInternal`, before the component download begins.

#### `internal/exec/vendor_utils.go`

Calls `warnIfArchivedGitHubRepo` for remote package types in `processAtmosVendorSource`, before processing vendor manifest targets.

### Decision Flow

```
vendor pull triggered
        │
        ▼
determinePackageType()
        │
   pkgTypeRemote? ──No──► skip check
        │
       Yes
        │
        ▼
ParseGitHubOwnerRepo(uri)
        │
    ok=false? ───────────► skip check (non-GitHub source)
        │
      ok=true
        │
        ▼
IsRepoArchived(owner, repo)
        │
     error? ────────────► silently skip (API unavailable)
        │
      false ────────────► no warning, proceed
        │
      true
        │
        ▼
   log.Warn(...)          ← warning emitted
        │
        ▼
  vendoring proceeds
```

### Authentication and Rate Limits

The GitHub API has a public rate limit of 60 requests per hour for unauthenticated callers. Setting `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN` raises this to 5,000 requests per hour.

In CI environments without a token, the check will succeed for small vendor manifests but may be silently skipped (rate limited) for large ones. This is acceptable: the check is best-effort by design.

---

## Files Modified

| File | Change |
|------|--------|
| `pkg/github/repo.go` | New file: `ParseGitHubOwnerRepo` and `IsRepoArchived` functions |
| `pkg/github/repo_test.go` | New file: 22 unit tests for `ParseGitHubOwnerRepo` covering all URI formats |
| `internal/exec/vendor_github_archive.go` | New file: `warnIfArchivedGitHubRepo` helper |
| `internal/exec/vendor_component_utils.go` | Call `warnIfArchivedGitHubRepo` for remote component sources |
| `internal/exec/vendor_utils.go` | Call `warnIfArchivedGitHubRepo` for remote vendor manifest sources |
| `website/blog/2026-03-12-warn-vendor-archived-repo.mdx` | Blog post / changelog entry |
| `website/src/data/roadmap.js` | Roadmap milestone added to Source Provisioner initiative |

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

The existing golden snapshot tests exercise the vendor pipeline end-to-end. Because `IsRepoArchived` makes a live GitHub API call and CI does not provide a real GitHub token, the check is silently skipped in CI (returns an error, which is swallowed). This ensures that the warning feature does not break any existing snapshot tests.

---

## Alternatives Considered

### Block vendoring on archived repositories

Treating an archived dependency as a fatal error would prevent outdated code from entering the codebase. However, this would break existing workflows for teams that intentionally vendor from archived repos (e.g., a stable, unmaintained-but-functional module). A warning achieves the same awareness goal without disrupting existing pipelines.

### Check only on first vendor, not on re-vendor

Caching the archived status locally (e.g., in workdir metadata) would reduce API calls. This adds state management complexity and prevents engineers from seeing the warning when a previously-unarchived repo becomes archived. The current implementation re-checks on every `vendor pull`, which is the simplest approach and consistent with the existing model where vendoring is the signal to inspect the source.

### Require explicit opt-in

Making the check opt-in (via a flag or config key) reduces noise for teams that don't care about archive status. However, the target audience — teams adopting infrastructure-as-code best practices — benefits from the warning by default, and the warning is low-noise (shown once per source, only when relevant).

---

## References

- GitHub Issue: #2046
- GitHub Pull Request: #2175
- Blog post: `website/blog/2026-03-12-warn-vendor-archived-repo.mdx`
- GitHub REST API — Get a repository: https://docs.github.com/en/rest/repos/repos#get-a-repository
- Existing GitHub client: `pkg/github/client.go`
- Vendor URI normalization PRD: `docs/prd/vendor-uri-normalization.md`
