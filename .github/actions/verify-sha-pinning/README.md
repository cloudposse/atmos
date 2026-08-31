# Verify SHA Pinning

Runs two checks against every third-party `uses:` reference in workflow files:

- **Coverage** — is the reference SHA-pinned at all?
- **Drift** — if it's pinned to a tag, does the SHA still match what that tag resolves to upstream?

## Why

SHA pinning (e.g., `actions/checkout@de0fac2e... # v6.0.2`) is a supply chain security best practice — but only if every reference is actually pinned, and only if the SHA actually corresponds to the claimed tag. Attackers can [force-push tags to malicious commits](https://rosesecurity.dev/2026/03/20/typosquatting-trivy.html), making the version comment a lie while the SHA points to compromised code. A reference left on a bare tag (`@v4`) gets none of this protection at all — anyone who can push to that tag upstream controls what runs in CI.

This action catches:
- **Unpinned references** — a `uses:` line with no SHA at all (just a floating tag or branch)
- **SHA/tag mismatch** — pinned SHA doesn't match what the upstream tag resolves to
- **Stale pins** — tag was moved upstream (force-push) after initial pinning
- **Typosquatting** — tag not found because the owner/repo is wrong

On a tag mismatch, the action investigates whether the pinned SHA exists in the claimed repo and what tags (if any) it corresponds to — helping distinguish stale pins from supply chain attacks.

### Branch-pinned references

Some upstream repos don't tag the thing being referenced (e.g. a reusable workflow call, or an action whose maintainer doesn't cut releases). For these, pin to a specific commit SHA on the tracked branch with a comment naming the branch instead of a version, e.g.:

```yaml
uses: hashicorp/setup-packer@ce93c3c08a6c2ff2275bf4b54ff0d9a75f6c9789 # main
uses: cloudposse/.github/.github/workflows/shared-go-auto-release.yml@49ac8cd5c4cf74abdbf24a80027cd3bde811133a # main
```

This satisfies the coverage check (a specific commit is nailed down — it can't be silently swapped by a force-push) but is intentionally excluded from drift-checking, since there's no tag to diff against. It's reported as an informational `pinned-branch` status, not a failure. Bumping a branch-pinned reference to a newer commit on that branch is a manual, deliberate action — there's no automated staleness check for it today.

### Allowlist (`allowlist.json`)

Some upstream orgs block the GitHub API calls this action needs to resolve a tag — most commonly an org-level [IP allow list](https://docs.github.com/en/organizations/keeping-your-organization-secure/managing-security-settings-for-your-organization/restricting-network-traffic-to-your-organization) that hasn't been opened up for GitHub Actions runner IPs. That's an access failure, not a drift signal, but the two are indistinguishable to CI unless someone says so explicitly.

`allowlist.json` is that explicit, human-reviewed record. Each entry names an `owner/repo`, a `description` a maintainer has personally verified (not inferred from whatever error text the API happened to return), and `references` to corroborating reports:

```json
[
  {
    "action": "aquasecurity/trivy-action",
    "description": "...",
    "references": ["https://github.com/aquasecurity/tfsec-action/issues/24"]
  }
]
```

**This is the only mechanism that downgrades a resolution failure to a warning, and only for the specific access-block condition (an HTTP 403) the entry documents.** A listed repo's tag lookup can still fail hard: a 404 (deleted/renamed tag), a malformed API response, or an exhausted retry on a transient error is never downgraded, even for a listed repo — only a 403 is. Not every 403 counts, either: a rate-limit 403 (`x-ratelimit-remaining: 0` on the response) means *this token* is out of budget for every remaining lookup, not that this one repo is access-blocked, so it's excluded and stays a hard failure even for a listed repo — detected via that response header, never by inspecting the error's message text. A listed repo still goes through the normal tag-resolution attempt on every run; if the API call ever succeeds, normal drift-checking applies and can still fail on a genuine mismatch. Adding or removing an entry requires a reviewed PR to this file, and every entry must have a non-empty `action`, `description`, and at least one `references` entry — a malformed entry fails the whole check rather than silently suppressing a real failure.

### Trust boundary

This action's own code and `allowlist.json` are read from the PR's own head, the same way gitleaks/git-secrets read their allowlist config from the diff being scanned — an allowlist you can't extend from the PR that needs the extension isn't useful. Protection against a self-serving allowlist entry (or a patched verifier that always passes) comes from required review before merge, not from hiding the policy behind a base-branch checkout: `main` requires at least one approval, requires a code-owner review, and dismisses stale approvals whenever new commits are pushed, so a reviewer always sees the actual diff — including any change to this action or `allowlist.json` — before it can merge.

## Usage

```yaml
permissions:
  contents: read
  pull-requests: write

steps:
  - uses: actions/checkout@v6
  - uses: ./.github/actions/verify-sha-pinning
    with:
      github-token: ${{ secrets.GITHUB_TOKEN }}
```

**Note:** `pull-requests: write` is required for posting sticky PR comments.

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `github-token` | Yes | — | GitHub token for API calls and PR comments |
| `workflow-dir` | No | `.github/workflows` | Directory to scan |
| `allowlist-file` | No | `.github/actions/verify-sha-pinning/allowlist.json` | Allowlist file (see above) |

## Outputs

| Output | Description |
|--------|-------------|
| `verified-count` | Number of tag-pinned actions verified against their upstream tag |
| `failed-count` | Number of drift mismatches or resolution errors found |
| `unpinned-count` | Number of third-party action references that are not SHA-pinned at all |
| `allowlisted-count` | Number of resolution failures downgraded to a warning via `allowlist.json` |
| `status` | `pass` or `fail` |

## PR Comments

On pull requests, the action posts a sticky comment (updated in place):
- **Failure**: Warning table listing every unpinned reference and drift mismatch, with forensic details
- **Resolved**: Updated to show all references covered (SHA-pinned) and all tag-pins verified

No comment is posted on clean PRs that have never had a violation.

## What it scans

Every `uses:` line in `*.yml` / `*.yaml` (skipping local composite/action refs like `uses: ./...`), classified as:

```
uses: owner/repo[/sub]@<40-char-sha> # v<tag>   → tag-pinned, drift-checked
uses: owner/repo[/sub]@<40-char-sha> # <branch> → branch-pinned, coverage only (see above)
uses: owner/repo[/sub]@<tag-or-branch>          → unpinned — the coverage gap this action exists to catch
```

Handles sub-actions and reusable workflow calls (`owner/repo/sub@sha # tag`), tag comments with or without a leading `v` (e.g. `# 0.1.1`), and both annotated and lightweight git tags.

## Local testing

```bash
GITHUB_TOKEN=$(gh auth token) node .github/actions/verify-sha-pinning/test.mjs
```
