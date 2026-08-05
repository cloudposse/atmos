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
uses: cloudposse/.github/.github/workflows/shared-go-auto-release.yml@8244c7c9142e92281e7841f655fa48e9ceb9b454 # main
```

This satisfies the coverage check (a specific commit is nailed down — it can't be silently swapped by a force-push) but is intentionally excluded from drift-checking, since there's no tag to diff against. It's reported as an informational `pinned-branch` status, not a failure. Bumping a branch-pinned reference to a newer commit on that branch is a manual, deliberate action — there's no automated staleness check for it today.

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

## Outputs

| Output | Description |
|--------|-------------|
| `verified-count` | Number of tag-pinned actions verified against their upstream tag |
| `failed-count` | Number of drift mismatches or resolution errors found |
| `unpinned-count` | Number of third-party action references that are not SHA-pinned at all |
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
