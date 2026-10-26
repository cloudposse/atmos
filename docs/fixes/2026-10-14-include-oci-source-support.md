# Fix: `!include` now actually supports `oci://` sources as documented

**Date:** 2026-10-14

## Summary

The `!include` YAML function's docs (`website/docs/functions/yaml/include.mdx`) documented
`oci://` as a supported remote source, with an example like
`!include oci://ghcr.io/my-org/my-image:path/to/vars.yaml`. In the actual implementation, `oci://`
sources were dispatched to the `go-getter`-based remote downloader, which had no `"oci"` entry in
its `Getters` map — every `!include oci://...` failed. The documented example was also an invalid
OCI reference on its own terms: an OCI tag cannot contain `/`, so `image:path/to/vars.yaml` can
never be parsed as `<repo>:<tag>` by `go-containerregistry`.

## Context

- `pkg/utils/yaml_include_by_extension.go`'s `processRemoteFile` routes every non-local include
  through `downloader.NewGoGetterDownloader`, whose `Getters` map only registered `git`, `file`,
  `hg`, `http`, and `https`. go-getter has no built-in concept of an OCI registry.
- An OCI artifact is pulled and extracted as a whole directory; there is no registry API to fetch
  a single layer-internal path. `!include`, unlike vendoring/scaffold, needs one specific file, so
  the fix needed a way to name a path inside the pulled image without colliding with the OCI
  reference's own `:tag` syntax.
- go-getter's built-in `//subdir` handling cannot do that selection for a file: when the subdir
  resolves to a single file it silently copies nothing (verified against go-getter v1.8.6, where
  `copyDir` skips its root entry). Only sub-directories work, so the file pick has to live in
  `!include` itself.
- How the image is pulled (the shared OCI getter) is covered separately in
  `2026-10-19-shared-oci-getter.md`.

## Changes

- `pkg/utils/yaml_include_by_extension.go`: `processRemoteFile` now detects `oci://` sources and
  routes them to a new `processOCIIncludeFile`, which:
  - Splits the include value into the OCI image reference and an in-image file path using
    go-getter's `"//"` subdirectory convention (`getter.SourceDirSubdir`, the same convention
    already used for Git/S3 sources in `pkg/vendor/uri.go`), e.g.
    `oci://ghcr.io/org/image:tag//path/to/file.yaml`. This is required, not just reused for
    consistency: since an OCI tag cannot contain `/`, there is no other way to fold a file path
    into the reference.
  - Requires a non-empty subdir; returns a dedicated error (`ErrIncludeYamlFunctionOCIMissingPath`)
    otherwise.
  - Pulls the image into a temp directory (cleaned up via `defer`) through the shared downloader,
    bounded by a 2-minute `ociIncludeTimeout` (shorter than vendoring's 10-minute budget, since
    `!include` targets a single small config file).
  - Resolves the requested file via `pkg/filesystem.SafeJoin` (guards against the subdir escaping
    the temp directory) and parses it with the existing `processLocalFile` (extension-based or raw,
    matching `!include`/`!include.raw`).
- `pkg/utils/yaml_utils.go`: added the `ErrIncludeYamlFunctionOCIMissingPath` sentinel error.
- `website/docs/functions/yaml/include.mdx`: corrected the `oci://` example to the working
  `oci://ghcr.io/my-org/my-image:v1.0.0//path/to/vars.yaml` syntax and explained why the `//` is
  required.

## Validation

- `go build ./...` — full repository build succeeds.
- `go test ./pkg/utils/...` — all tests pass, including three regression tests in
  `pkg/utils/yaml_include_by_extension_regression_test.go`, run against an in-process fake OCI
  registry (`pkg/oci/ocitest`):
  - `TestIncludeOCISource` — pulls a file out of the image via `!include oci://...//vars.yaml` and
    asserts the parsed YAML content.
  - `TestIncludeOCISourceMissingPath` — asserts the dedicated error when no `//` path is given.
  - `TestIncludeOCISourceFileNotFound` — asserts a clear error when the named path doesn't exist
    inside the pulled image.
- `atmos lint --changed` — 0 issues.

## Follow-ups

None.
