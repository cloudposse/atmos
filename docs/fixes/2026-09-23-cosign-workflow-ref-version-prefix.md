# Fix: cosign `--certificate-github-workflow-ref` keeps the tag's `v` prefix

**Date:** 2026-09-23

## Summary

Toolchain installs began failing cosign signature verification for tools whose GitHub release tag
is `v`-prefixed (e.g. `terraform-linters/tflint`, `charmbracelet/gum`):

```
Error: expected GitHub Workflow Ref not found in certificate
  --certificate-github-workflow-ref refs/tags/0.64.0   (should be refs/tags/v0.64.0)
```

`renderArgs` now aligns the bare (non-URL) cosign `--certificate-github-workflow-ref` value with the
tool's **actual downloaded asset tag**, the same way the existing URL correction already aligns
`--certificate-identity`. This handles the case where Atmos's rendered `{{.Version}}` disagrees with
the tag of the asset it fetched. Fixes cloudposse/atmos#3209.

This is a targeted, symptomatic fix. The underlying divergence between Atmos's version formatting and
the aqua registry's canonical tag format is tracked separately in #3211.

## Context

The trigger was an upstream change to the **unpinned** `aquaproj/aqua-registry` `main` branch that
Atmos resolves package metadata from: on 2026-09-23 upstream **added** a
`--certificate-github-workflow-ref refs/tags/{{.Version}}` assertion to the tflint (10:59 UTC) and
gum (07:05 UTC) cosign configs. CI runs before those commits passed; runs after them failed - which
is why it "worked yesterday" and broke today, with no Atmos code change involved. It is not a cold
cache: both the last passing run and the failing runs got a toolchain cache hit and both executed
cosign.

Note: the upstream template is not itself wrong. `{{.Version}}` is meant to be the tool's actual
release tag, and tflint's registry entry has no `version_prefix` (per aqua semantics that means the
tag is used as-is, `v` and all). The failure comes from **Atmos rendering `{{.Version}}` v-stripped
(`0.64.0`) while the real release tag - and the asset Atmos actually downloaded - is `v0.64.0`**.
That divergence originates in Atmos's version handling (`normalizeGitHubVersion` strips the `v` when
registry metadata is unavailable; the download fallback re-adds it on a 404), not in the aqua
template. The new `--certificate-github-workflow-ref` assertion simply made a previously-harmless
divergence fatal.

`renderArgs` already realigned URL args (`--certificate-identity`) to the downloaded asset tag via
`replaceVersionSegmentInURL`, which only rewrites URL-shaped values (those with a host). The bare
`--certificate-github-workflow-ref refs/tags/0.64.0` is not a URL, so it was left v-stripped and no
longer matched the signing certificate's `refs/tags/v0.64.0`; cosign then rejected a valid signature.

This fix does not attempt to establish which exact resolution/download path produced the v-stripped
version in the reported live run; it aligns the workflow-ref with the downloaded asset tag so the
mismatch cannot break verification. The canonical fix - making the aqua registry `version_prefix` and
the real tag the single source of truth for `{{.Version}}`, and removing the `v`-prefix heuristics -
is #3211.

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

- **#3211** - the canonical fix: make the aqua registry `version_prefix` and the real release tag the
  single source of truth for `{{.Version}}`, and remove the `v`-prefix heuristics
  (`normalizeGitHubVersion` stripping `v` when metadata is unavailable; the 404 download fallback that
  guesses a prefix). Once that lands, this PR's workflow-ref alignment becomes a no-op safety net.
- Broader hardening (noted on #3209): pin the aqua registry to a reviewed ref instead of tracking
  upstream `main` unpinned, so an upstream registry change cannot silently break toolchain installs.
