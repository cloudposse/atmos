# Release publication guard

Pins a draft's identity, uploads verified assets and publishes, or verifies an
already published release. `actions/github-script` loads `publish.cjs`; the YAML
only supplies inputs and calls its tested `run()` entrypoint.

| Input | Required | Purpose |
| --- | --- | --- |
| `operation` | Yes | `prepare`, `provenance`, `publish`, or `verify`. |
| `github-token` | Yes | Contents write for prepare/publish; contents read for verify. |
| `release-id` | Yes | Exact numeric GitHub draft or published release ID. |
| `commit` | For prepare/provenance/publish | Full source commit SHA. |
| `tag` | For provenance/publish | Authoritative tag returned by prepare. |
| `artifact-directory` | For publish | Directory sealed by `release:verifyArtifacts`. |
| `provenance-file` | For provenance | Output path for the SLSA predicate. |

`prepare` outputs `id`, `tag`, and `commit`. It requires an empty draft, creates
an absent tag at the requested commit, rejects conflicting existing tags, and
sets the draft target to that exact commit.

The selected source commit may belong to a maintenance branch for a hotfix;
stable releases do not require ancestry from the default branch. Selecting the
source is an authorized release operator's decision; the guard requires the
tag and every binary to match it exactly. The manual publishing
workflow itself runs only from the default branch so it uses trusted tooling.

`publish` uploads only the manifest's files. Immediately before publication it
checks every uploaded asset's GitHub SHA-256 digest, the complete asset set,
draft identity and remote tag commit. Failures leave a draft. It never deletes
or overwrites existing tags or assets. The caller must have completed the
immutable-release cutover and hold release concurrency throughout preparation,
build and publication; normally use the `release-pipeline.yml` workflow rather than invoking the
publishing operation directly.

`provenance` records the selected source commit separately from the workflow
revision. `verify` requires an immutable published release whose tag, manifest
and uploaded asset digests agree. It is the downstream distribution gate:

```yaml
- uses: $/.github/actions/release-publish
  with:
    operation: verify
    github-token: ${{ github.token }}
    release-id: ${{ github.event.release.id }}
```

The caller hardens the runner first. No checkout is needed just to load this
action. The manifest provides consistency checking; it does not prevent a
privileged direct publisher from bypassing the workflow or forging a manifest.

From the repository root:

```sh
node --test .github/actions/release-publish/publish.test.cjs
go test -tags=mage -race ./magefiles/release ./internal/releaseidentity
```

Tests use an in-memory GitHub client and real temporary binary fixtures. They
never publish a live release.
