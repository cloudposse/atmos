# Fix: sign-and-attest release job failed re-publishing a published release (422)

**Date:** 2026-09-06

## Summary

Publishing `v1.228.0` fired `build.yml`'s `sign-and-attest-release` job
("Rebuild, sign, SBOM, and attest release artifacts"). GoReleaser rebuilt every
platform and cataloged all SBOMs successfully, then failed at the very end:

```text
release failed after 53m45s
  error=scm releases: failed to publish artifacts: could not release:
  PATCH https://api.github.com/repos/cloudposse/atmos/releases/383707212:
  422 Validation Failed [{Resource:Release Field:target_commitish Code:invalid}]
```

## Root cause

This job runs on `release: published`, i.e. **after** the GitHub Release already
exists (with its assets, from the draft build). It ran `goreleaser release
--clean`, which includes the publish phase: GoReleaser tries to update the
existing release, sending `target_commitish`. GitHub rejects changing
`target_commitish` on an already-published release, returning `422 Validation
Failed`, so GoReleaser exits non-zero and the job fails.

The job does not actually need to re-publish - it rebuilds `dist/*` locally only
so the following steps can attest build provenance (`subject-path: dist/*`) and
generate a source-tree SBOM. The publish attempt was pure collateral.

## Fix

**`.github/workflows/build.yml`** - add `--skip=publish` to the GoReleaser args
in the `sign-and-attest-release` job:

```yaml
args: release --clean --skip=publish
```

GoReleaser still builds, signs, and catalogs SBOMs into `dist/*` (what the
attestation steps consume) but no longer touches the GitHub Release, so the 422
cannot occur.

## Notes

- The release itself was never at risk: the draft build already attached the
  cosign-signed checksums and per-artifact SBOMs. Only the extra GitHub-native
  build-provenance attestation was missing when this job failed.
- This was the job's first real run - it was added in #2958 (2026-08-31), after
  the previous publish (v1.227.0, 2026-08-26), and the earlier prerelease
  failures were a different symptom (missing binaries from the syft break, see
  #3061).
