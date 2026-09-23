# Fix: cosign `--certificate-github-workflow-ref` keeps the tag's `v` prefix

**Date:** 2026-09-23

## Summary

Toolchain installs began failing cosign signature verification for tools whose GitHub release tag
is `v`-prefixed (e.g. `terraform-linters/tflint`, `charmbracelet/gum`):

```
Error: expected GitHub Workflow Ref not found in certificate
  --certificate-github-workflow-ref refs/tags/0.64.0   (should be refs/tags/v0.64.0)
```

The version-segment correction in `renderArgs` now applies to non-URL cosign args too, so the
rendered `--certificate-github-workflow-ref` carries the tool's actual `v`-prefixed release tag.
Fixes cloudposse/atmos#3209.

## Context

Atmos resolves aqua package metadata live from the **unpinned** upstream `aquaproj/aqua-registry`
`main` branch. On 2026-09-23 upstream added `--certificate-github-workflow-ref refs/tags/{{.Version}}`
to the tflint (10:59 UTC) and gum (07:05 UTC) cosign configs. CI runs before those commits passed;
runs after them failed - which is why it "worked yesterday" and broke today, with no Atmos code
change involved. It is not a cold cache: both the last passing run and the failing runs got a
toolchain cache hit and both executed cosign.

The upstream template uses `{{.Version}}` for both `--certificate-identity` (a URL) and
`--certificate-github-workflow-ref` (a bare ref). In `pkg/toolchain/verification`, `{{.Version}}`
renders v-stripped (`0.64.0`) while the tool's real release tag is `v`-prefixed. `renderArgs`
corrected the version segment via `replaceVersionSegmentInURL`, which only rewrites URL-shaped
values (those with a host). So `--certificate-identity` was corrected to `v0.64.0`, but the bare
`--certificate-github-workflow-ref refs/tags/0.64.0` was left v-stripped and no longer matched the
signing certificate's `refs/tags/v0.64.0`. cosign then rejected a valid signature. The defect was
latent and exposed by the upstream registry change.

## Changes

- `pkg/toolchain/verification/checksum.go`: added `replaceVersionSegmentInPath`, the non-URL
  counterpart of `replaceVersionSegmentInURL`. It rewrites `/`-delimited version segments in a bare
  value (e.g. `refs/tags/0.64.0` -> `refs/tags/v0.64.0`) using the effective release tag, and leaves
  URL-shaped values to `replaceVersionSegmentInURL`.
- `pkg/toolchain/verification/signature.go`: `renderArgs` applies the URL corrector to every arg and
  the non-URL corrector **only to the `--certificate-github-workflow-ref` value** (identified by its
  preceding flag). Scoping it to that one option avoids rewriting a literal version segment in an
  unrelated non-URL option (e.g. a `--key /keys/0.64.0/public.pem` path). The previous arg is tracked
  in a local variable rather than indexing `args[i-1]` (avoids a gosec G602 false positive).
- `pkg/toolchain/verification/checksum_test.go`: `TestReplaceVersionSegmentInPath` (unit cases incl.
  bare ref, no-version, URL-passthrough, and no-op guards) and `TestRenderArgsCorrectsCosignWorkflowRef`
  (reproduces #3209: both the identity URL and the bare workflow-ref render with the `v`-prefixed tag).

## Validation

```bash
# Reproduction test fails before the fix (workflow-ref rendered as refs/tags/0.64.0), passes after.
go test ./pkg/toolchain/verification/ -run 'TestRenderArgsCorrectsCosignWorkflowRef|TestReplaceVersionSegmentInPath' -v

# No regressions in the verification package.
go build ./...
go test ./pkg/toolchain/verification/
atmos lint --changed
```

All listed tests pass; `atmos lint --changed` reports 0 issues.

## Follow-ups

- #3209 also notes a broader hardening: pin the aqua registry to a reviewed ref instead of tracking
  upstream `main` unpinned, so an upstream registry change cannot silently break toolchain installs.
  Tracked in that issue for a separate change.
