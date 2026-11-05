# Release PR notifications

Preserves the notification logic previously in Cloud Posse's
`shared-release-branches.yml`: compare the published release with the preceding
release and comment on each associated pull request with the release link.
The script runs through `actions/github-script`; it does not require a checkout.

Call this action after release verification, in a workflow triggered by
`release: published`, with `contents: read` and `pull-requests: write` permissions:

```yaml
- uses: $/.github/actions/release-comment
  with:
    github-token: ${{ github.token }}
```

`github-token` is the only input and is required. The action has no outputs.

Run the offline regression tests with:

```sh
node --test .github/actions/release-comment/comment.test.cjs
```

`atmos test local-actions javascript` and the local action CI workflow discover
these tests automatically. Tests cover PR deduplication, exact release links,
missing previous releases, empty comparisons, and API failures.
