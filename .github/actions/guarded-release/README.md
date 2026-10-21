# Guarded release

The common build, verification, upload and publication action for stable,
nightly and feature releases. Its inputs explicitly identify the draft, source
commit, release bot credentials and repository activation flag.

| Input | Required | Purpose |
| --- | --- | --- |
| `release-id` | Yes | Existing empty GitHub draft release ID. |
| `commit` | Yes | Full source commit SHA. |
| `app-id` | Yes | Release bot GitHub App ID. |
| `app-private-key` | Yes | Release bot App private key. |
| `releases-enabled` | Yes | Pass `vars.ATMOS_GUARDED_RELEASES_ENABLED`; must be `true`. |

The action has no outputs. Publication is its final side effect.

Call it with `uses: $/.github/actions/guarded-release`. The calling job must use
an Ubuntu runner, the appropriate protected release environment, a 180-minute
timeout, and `contents: read`, `id-token: write`, `attestations: write`
permissions. Its workflow must hold the `atmos-release` concurrency group with
`cancel-in-progress: false` across drafting and publication.

The calling workflow must harden the runner as its first step, before invoking
this action. The action checks out verification tooling and source separately,
creates short-lived contents-write App tokens before
preparation and publication, and delegates API operations to the tested
`release-publish/publish.cjs` module through `actions/github-script`.

Run its verification and publication regression tests from the repository root:

```sh
go test -tags=mage -race ./magefiles/release ./internal/releaseidentity
node --test .github/actions/release-publish/publish.test.cjs
```

See [the incident analysis and cutover instructions](../../../docs/fixes/release-tag-artifact-integrity.md).
