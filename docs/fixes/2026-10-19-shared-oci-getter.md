# Fix: route every `oci://` pull through one shared OCI getter

**Date:** 2026-10-19

## Summary

Four consumers each pulled OCI artifacts by calling `pkg/oci.ProcessImage` directly, each with its
own copy of the `oci://` dispatch: component vendoring, scaffold template sourcing, component
source provisioning, and (newly) the `!include` YAML function. go-getter has no OCI scheme, so none
of them went through the shared go-getter downloader the way every other protocol does. A review of
the `!include` change asked for a single shared path instead of another per-consumer carve-out.

## Context

- The shared downloader (`pkg/downloader/gogetter_downloader.go`) builds a `getter.Client` with a
  `Getters` map (`git`, `file`, `hg`, `http`, `https`, `s3`). There was no `"oci"` entry.
  `CustomGitGetter` (`pkg/downloader/git_getter.go`) was the existing precedent for a custom getter.
- go-getter's `Getter` interface is `Get`, `GetFile`, `ClientMode`, `SetClient`. `Client.Configure`
  calls `SetClient` on every registered getter, which is how a getter reaches the caller's context
  (the methods take no `context.Context`).
- An OCI artifact is a set of layers extracted into a directory, so it only fits directory mode.
- go-getter's own `//subdir` selection copies directories only; a subdir that resolves to a single
  file silently copies nothing (verified against go-getter v1.8.6). Single-file selection therefore
  stays in `!include` (see `2026-10-14-include-oci-source-support.md`).
- Reference parsing: `oci://host:port/repo:tag` and digest refs parse as URLs, but a host-less ref
  such as `oci://ubuntu:latest` puts `latest` in the port slot and fails `url.Parse`.

## Changes

- `pkg/downloader/oci_getter.go` (new):
  - `OCIGetter`, a `getter.Getter` registered as `"oci"`. `Get` extracts the image into the
    destination through `pkg/oci.ProcessImage`, using the client's context for cancellation and
    retry observation. `ClientMode` always reports directory mode; `GetFile` returns
    `ErrOCIGetFileUnsupported`.
  - `prepareOCISource`, called from `goGetterClientFactory.NewClient` next to
    `applyForcedGitDetection`. It normalizes an `oci://` source with `name.ParseReference` (so
    host-less refs work), keeps any `//subdir`, and adds `?archive=false` so go-getter never treats
    a ref ending in an archive suffix as an archive to decompress. Because this lives in the
    factory, callers pass `oci://...` to the ordinary `Fetch`/`FetchWithMetadataContext` methods
    like any other protocol; there is no OCI-specific entry point.
- `pkg/downloader/gogetter_downloader.go`: registers the `"oci"` getter, and `isRemoteSource`
  excludes `oci://` so the git/GitHub credential brokers are not triggered for registry pulls
  (registries authenticate through go-containerregistry: docker keychain, ghcr.io token).
- The per-caller `oci://` branches were removed in favor of the generic fetch path:
  - `pkg/provisioner/source/vendor.go`: deleted `downloadOCISource` and the `IsOCIURI` branch;
    `downloadSource` handles every protocol the same way.
  - `pkg/vendoring/install/fetch.go`: deleted `fetchOCI`; the `PkgTypeOci` case reuses
    `fetchRemote` (directory mode, fetched straight into the temp dir) and still wraps failures
    with `ErrProcessOCIImage`.
  - `pkg/generator/source/resolver.go`: `fetchOCI` remains only because it resolves and pins the
    manifest digest first (recording `Directory.ResolvedRef`) and keeps its OCI-specific hints on
    `ErrScaffoldFetchSource`; the pull itself goes through the shared downloader.
  - `pkg/utils/yaml_include_by_extension.go` (`!include`): pulls through the shared downloader.
- `errors/errors.go`: added `ErrOCIGetFileUnsupported`.
- `website/docs/functions/yaml/include.mdx`: describes the shared `oci` getter.

## Validation

- `go build ./...` and `go vet` on the touched packages — clean.
- `go test` for `pkg/downloader`, `pkg/utils`, `pkg/vendoring/...`, `pkg/generator/...`,
  `pkg/provisioner/...`, `pkg/stack/imports`, `pkg/oci/...` — pass.
- CLI-level OCI tests in `tests/` (`TestScaffoldGenerate_OCISource`,
  `TestScaffoldGenerate_OCISourceUpdate`, `TestInit_OCISource`, `TestJITSource_OCIScheme`) — pass.
- New tests, all against the in-process fake registry (`pkg/oci/ocitest`):
  - `pkg/downloader/oci_getter_test.go`: `Get` extracts the image; works without a client;
    honors a cancelled context; `GetFile` returns the sentinel; `ClientMode` is directory; an
    `oci://` source fetched through the ordinary downloader, including `//subdir` directory
    selection, an invalid reference, and a pull failure keeping `ErrPullImage`;
    `prepareOCISource` normalization cases; `isRemoteSource` does not treat `oci://` as a
    broker-gated remote.
  - `pkg/vendoring/install/fetch_oci_test.go`: the `PkgTypeOci` path of `fetchToTempDir` succeeds,
    and a failure wraps both `ErrProcessOCIImage` and `ErrPullImage`.
- `atmos lint --changed` — 0 issues.

## Follow-ups

None.
