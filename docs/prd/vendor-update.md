# PRD: `atmos vendor update` & `atmos vendor diff`

## Status

Implemented. Supersedes the older `feat/vendor-diff-and-update` branch — this version
reuses the format-preserving YAML engine (`pkg/yaml`) and the existing vendoring
helpers instead of re-porting that branch's ~7k lines.

## Problem

Keeping vendored components up to date means manually checking upstream Git
repositories for new releases and hand-editing `version:` fields in `vendor.yaml`
(and its imports) — error-prone, and easy to clobber comments/anchors. There is
also no way to preview what changed between two versions before adopting one.

## Goals

1. **`atmos vendor update`** — check Git sources for a newer allowed version and
    update the `version` field in place, **preserving comments, anchors, and
    templates** (`{{.Version}}` in source URLs). Dry-run with `--check`.
2. **`atmos vendor diff`** — show the Git diff between two versions (tags,
    branches, or commits) of a vendored component, without a local checkout.
3. **Version constraints** — per-source semver constraints, exclusions, and a
    no-prereleases toggle.

## Non-goals (initial version)

- Version detection for OCI / S3 / GCS / HTTP sources (Git only — `git ls-remote`
  tags cover GitHub/GitLab/Bitbucket/self-hosted uniformly).
- Private-repository auth for tag listing / cloning (public repos for now).
- A bespoke Bubble Tea progress UI (uses the standard `ui.*` output).

## Design — built on what already exists

| Need | Reused / new |
| --- | --- |
| Format-preserving `version:` write | **Reuse** `pkg/yaml` + `pkg/vendoring.SetComponentVersion` (matches the source by component name via a yq `select()`), so no bespoke yaml.v3 walker. |
| Remote tag listing | **New, go-git** `version.GoGitLister` using `Remote.ListContext` — no shelling out to the `git` binary (cross-platform), behind a `RemoteLister` interface for testing. |
| Ref-to-ref diff | **New, go-git** `GoGitDiffer`: clone (bare, all tags, no checkout) into a temp dir, resolve both refs, `Commit.Patch` → unified diff. Tolerates a missing/extra leading `v`. |
| Semver + constraints | **Reuse** the existing `Masterminds/semver/v3` dependency; `version` subpackage filters by constraint, exclusions (incl. `1.5.*` wildcards), and prereleases. |
| Source loading / per-file editing | Read each manifest file (and its `imports:`) with a minimal decode and edit the file that **declares** each source, so imported files get updated correctly. |

The orchestration (`pkg/vendoring.Update` / `Diff`) is free of `internal/exec`;
the `cmd/vendor` layer resolves the manifest file(s) and calls it.

## Version constraints schema

```yaml
sources:
  - component: "vpc"
    source: "github.com/cloudposse/terraform-aws-components"
    version: "1.323.0"
    constraints:
      version: "^1.0.0"            # Masterminds/semver constraint
      excluded_versions:           # exact values or wildcard patterns
        - "1.2.3"
        - "1.5.*"
      no_prereleases: true         # skip alpha/beta/rc
```

Resolution pipeline: list tags → filter by `version` constraint → drop
`excluded_versions` → drop prereleases (if `no_prereleases`) → select the latest,
preferring the more specific tag on equal precedence (e.g. `v3.0.0` over `v3`).

The `constraints` block is added to `schema.AtmosVendorSource`
(`VendorConstraints`) and to the vendor JSON schema
(`pkg/datafetcher/schema/vendor/package/1.0.json`).

## Commands

```bash
atmos vendor update [--check] [--pull] [--component <name>] [--tags a,b] [--outdated]
atmos vendor diff --component <name> [--from <ref>] [--to <ref>] [--diff-file <path>]
```

For CI branch/PR publishing, scoped update groups, and GitHub step summaries, see the
[Native Component Updater PR Workflow](./component-updater.md). The core update/diff
semantics in this document remain the local primitive used by that workflow.

- `update --check` is a dry run; `--pull` runs `atmos vendor pull` after writing;
  `--outdated` shows only sources with an available update.
- `diff` defaults `--from` to the source's current pinned version and `--to` to
  the latest tag.

Sources are skipped (and reported) when the version is templated (`{{…}}`) or the
source is not a Git repository.

## Testing

- `pkg/vendoring/version` — table-driven constraint/semver/URI tests, no network.
- `pkg/vendoring` — `Update` with a fake `RemoteLister` (apply, dry-run, filters,
  constraints) asserting comments/templates survive; `Diff` with a mock
  `GitDiffer` (default-to-latest, non-Git rejection, file filter).
- Verified E2E against a public repo (tag listing, constraint resolution,
  format-preserving write, and a real ref-to-ref clone+diff).

## Known limitations

- Public Git repositories only (no auth for private tag listing / cloning yet).
- `vendor diff` performs a full clone into a temp directory (removed after); large
  repositories take proportionally longer.
- Blank lines between manifest entries are not preserved (inherent to the yaml.v3
  node model the editor builds on).
