# Verify SHA Pinning

Verifies that SHA-pinned GitHub Actions in workflow files match the tag claimed in their version comment.

## Why

SHA pinning (e.g., `actions/checkout@de0fac2e... # v6.0.2`) is a supply chain security best practice — but only if the SHA actually corresponds to the claimed tag. Attackers can [force-push tags to malicious commits](https://rosesecurity.dev/2026/03/20/typosquatting-trivy.html), making the version comment a lie while the SHA points to compromised code.

This action catches:
- **SHA/tag mismatch** — pinned SHA doesn't match what the upstream tag resolves to
- **Stale pins** — tag was moved upstream (force-push) after initial pinning
- **Typosquatting** — tag not found because the owner/repo is wrong

On mismatch, the action investigates whether the pinned SHA exists in the claimed repo and what tags (if any) it corresponds to — helping distinguish stale pins from supply chain attacks.

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
| `verified-count` | Number of SHA-pinned actions verified |
| `failed-count` | Number of mismatches found |
| `status` | `pass` or `fail` |

## PR Comments

On pull requests, the action posts a sticky comment (updated in place):
- **Failure**: Warning table with each action's status and forensic details
- **Resolved**: Updated to show all pins verified

No comment is posted on clean PRs that have never had a violation.

## What it scans

Any line in `*.yml` / `*.yaml` matching:

```
uses: owner/repo@<40-char-sha> # v<tag>
```

Handles sub-actions (`owner/repo/sub@sha # tag`) and both annotated and lightweight git tags.

## Local testing

```bash
GITHUB_TOKEN=$(gh auth token) node .github/actions/verify-sha-pinning/test.mjs
```
