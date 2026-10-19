# Fix: `!include` now actually supports `oci://` sources as documented

**Date:** 2026-10-14

## Summary

The `!include` YAML function's docs (`website/docs/functions/yaml/include.mdx`) documented
`oci://` as a supported remote source, with an example like
`!include oci://ghcr.io/my-org/my-image:path/to/vars.yaml`. In the actual implementation, `oci://`
sources were dispatched to the `go-getter`-based remote downloader, which has no `"oci"` entry in
its `Getters` map — every `!include oci://...` failed. The documented example was also an invalid
OCI reference on its own terms: an OCI tag cannot contain `/`, so `image:path/to/vars.yaml` can
never be parsed as `<repo>:<tag>` by `go-containerregistry` (the library already used elsewhere in
this repo for OCI pulls).

## Context

- `pkg/utils/yaml_include_by_extension.go`'s `processRemoteFile` routes every non-local include
  through `downloader.NewGoGetterDownloader`, whose `Getters` map
  (`pkg/downloader/gogetter_downloader.go`) only registers `git`, `file`, `hg`, `http`, and `https`.
  There is no `"oci"` getter, and go-getter has no built-in concept of an OCI registry.
- Elsewhere in the repo, every existing `oci://` consumer (component vendoring, scaffold template
  sourcing, component source provisioning) called `pkg/oci.ProcessImage` directly instead of going
  through `go-getter`, each with its own copy of the dispatch. A maintainer review on the PR asked
  for one shared path instead of another per-consumer carve-out for `!include`.
- `pkg/oci.ProcessImage` pulls an entire OCI image and extracts all of its layers into a directory;
  there is no registry API to fetch a single layer-internal path on its own. `!include`, unlike
  vendoring/scaffold, needs one specific file, so the fix needed a way to name a path inside the
  pulled image without colliding with the OCI reference's own `:tag` syntax.

## Changes

- `pkg/downloader/oci_getter.go` (new): `OCIGetter`, a `getter.Getter` registered as `"oci"` in
  `goGetterClientFactory.NewClient`, backed by `pkg/oci.ProcessImage` (directory mode only), plus
  `FetchOCIContext`, the shared helper that normalizes the reference and fetches through the
  downloader. `isRemoteSource` excludes `oci://` so the git/GitHub credential brokers are not
  triggered for registry pulls. The vendoring, scaffold, and component-provisioning callers were
  migrated onto it, preserving their timeouts, error wrapping, hints, and digest pinning.
- go-getter's own `//subdir` selection only copies directories (a subdir that resolves to a single
  file silently copies nothing; verified against go-getter v1.8.6), so single-file selection stays
  in `!include`.
- `pkg/utils/yaml_include_by_extension.go`: `processRemoteFile` now detects `oci://` sources and
  routes them to a new `processOCIIncludeFile`, which:
  - Splits the include value into the OCI image reference and an in-image file path using
    `go-getter`'s own `"//"` subdirectory convention (`getter.SourceDirSubdir`) — the same
    convention already used for Git/S3 sources in `pkg/vendor/uri.go`, e.g.
    `oci://ghcr.io/org/image:tag//path/to/file.yaml`. This is required, not just reused for
    consistency: since an OCI tag cannot contain `/`, there is no other way to fold a file path
    into the reference.
  - Requires a non-empty subdir; returns a dedicated error (`ErrIncludeYamlFunctionOCIMissingPath`)
    otherwise.
  - Pulls the image via `pkg/oci.ProcessImage` into a temp directory (cleaned up via `defer`),
    bounded by a new 2-minute `ociIncludeTimeout` (shorter than vendoring's 10-minute budget, since
    `!include` targets a single small config file).
  - Resolves the requested file via `pkg/filesystem.SafeJoin` (guards against the subdir escaping
    the temp directory) and parses it with the existing `processLocalFile` (extension-based or raw,
    matching `!include`/`!include.raw`).
- `pkg/utils/yaml_utils.go`: added the `ErrIncludeYamlFunctionOCIMissingPath` sentinel error.
- `website/docs/functions/yaml/include.mdx`: corrected the `oci://` example to the working
  `oci://ghcr.io/my-org/my-image:v1.0.0//path/to/vars.yaml` syntax, and explained why the `//` is
  required and why OCI fetching takes a different code path than the other remote protocols.

## Validation

- `go build ./...` — full repository build succeeds.
- `go vet ./pkg/utils/...` — clean.
- `go test ./pkg/utils/...` — all tests pass (82.6% coverage), including three new regression
  tests in `pkg/utils/yaml_include_by_extension_regression_test.go`:
  - `TestIncludeOCISource` — pulls a real file out of an in-process fake OCI registry
    (`pkg/oci/ocitest`) via `!include oci://...//vars.yaml` and asserts the parsed YAML content.
  - `TestIncludeOCISourceMissingPath` — asserts the dedicated error when no `//` path is given.
  - `TestIncludeOCISourceFileNotFound` — asserts a clear error when the named path doesn't exist
    inside the pulled image.
- `atmos lint --changed` (via the built `./build/atmos` binary) — reports 2 pre-existing issues in
  `pkg/generator/engine/templating.go` and `pkg/generator/merge/text_merger.go`, neither touched by
  this change; no findings in the files this fix modified.

## Follow-ups

None.
