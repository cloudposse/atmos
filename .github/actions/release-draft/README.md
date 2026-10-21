# Draft release notes

Creates or updates a notes-only GitHub draft. Publication is hardcoded off;
building and publishing belong to `guarded-release`. This preserves the
existing Cloud Posse release-drafter configuration selection while making
credentials explicit in Atmos.

| Input | Required | Purpose |
| --- | --- | --- |
| `app-id` | Yes | Release bot GitHub App ID. |
| `app-private-key` | Yes | Release bot App private key. |
| `prerelease` | No | String `true` for nightly/feature drafts; defaults to `false`. |

Outputs `id` (draft release ID) and `exists` (whether the tag already exists).
No build version is calculated here for the publisher: the publisher reads
the selected draft's actual tag when preparation begins.

```yaml
- id: draft
  uses: $/.github/actions/release-draft
  with:
    app-id: ${{ vars.BOT_GITHUB_APP_ID }}
    app-private-key: ${{ secrets.BOT_GITHUB_APP_PRIVATE_KEY }}
    prerelease: 'false'
```

The caller hardens its runner first, selects the `release` or
`feature-releases` environment, and holds the `atmos-release` concurrency group
with cancellation disabled. The job's default token only needs `contents: read`;
the App token performs draft writes and reads pull requests. Feature callers
must enforce the same-repository and maintainer-label gates.

The action delegates drafting to pinned upstream actions. Its YAML is validated
alongside the calling workflows. Publisher regression tests live in
`../release-publish/publish.test.cjs`.
