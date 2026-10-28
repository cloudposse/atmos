# Fix: `oci://` imports now work in stack manifests and `atmos.yaml`

**Date:** 2026-10-28

## Summary

An `import:` of `oci://host/repo:tag//path/to/file.yaml` failed in both stack manifests and `atmos.yaml` with
"oci:// sources must be fetched in directory mode, not as a single file". Both import types now pull the
artifact as a directory and select the file, directory, or glob after `//`, the same way Git `//subdir`
imports work.

## Context

`oci://` was accepted as an import scheme (the config import adapter listed it, and `IsRemote()` treats any
`://` URI as remote), but the remote importer fetched every non-Git source in single-file mode. The OCI getter
only supports directory mode, because an OCI artifact is a set of layers. Only `!include`, vendoring, scaffold
sources, and Helm chart sources handled OCI correctly.

## Changes

- `pkg/stack/imports/remote.go`: an `oci://` import takes the same path as a Git subdirectory import. The
  artifact is pulled once into a shared source directory, then files are resolved and copied into the per-file
  cache. A missing `//path` returns the new `ErrOCIImportMissingPath` sentinel with a hint.
- `pkg/stack/imports/remote_nested.go`: `ensureSourceDir` skips the Git credential broker and Git detector for
  OCI sources and shares its reuse logic through `reuseOrFetchSourceDir`. `nested_imports: remote` accepts
  `oci://` sources.
- `pkg/stack/imports/uri.go`: new `IsOCIURI`, checked before `IsGitURI` because hosts such as `github.com` inside
  an `oci://` URI would otherwise be classified as Git.
- `resolveGitSubdir` and `resolveGitSubdirWithRemoteBase` are renamed to `resolveSourceSubdir` and
  `resolveSourceSubdirWithRemoteBase`, since they now serve both sources.
- `pkg/cache/file_cache.go`: the cached copy of a file now takes its extension from the fragment of the key
  (`uri#path/to/file.yaml`) when the URI part has none. Before, a directory or glob import such as
  `oci://host/repo:tag//.atmos.d` produced extensionless cache files, which `atmos.yaml` loading rejected with
  `Unsupported Config Type ""` and which lost `.tmpl` handling.
- `pkg/stack/imports/remote.go`: `cacheFile` no longer keeps the first cached copy of a file forever. Before, a
  mutable tag or `?ref=main` was re-pulled each run but the stale per-file copy was still served. Cached copies
  are now content-addressed (the key carries a digest of the file), so a new revision gets its own path, an
  unchanged file keeps its path, and a copy another `atmos` process is still reading is never rewritten under it.
  This shared helper also serves Git subdirectory imports, so they get the same correction. Superseded
  revisions stay in the XDG cache like other cached imports.
- `pkg/downloader/oci_credentials.go`: `atmos.yaml` imports are fetched while the configuration is still being
  assembled, before `settings` is populated from the environment, so a private `ghcr.io` artifact was pulled
  anonymously when the credentials came only from `GITHUB_TOKEN` and `GITHUB_ACTOR`. The OCI getter now falls back
  to the live environment for empty GHCR credential settings, the same way the Git detector does. Configured
  settings still win.
- Caching otherwise uses the same XDG cache (`stack-imports`) as other remote imports: one pull per
  invocation, cross-run reuse while `imports.ttl` (or the per-import `ttl`) is fresh, and a re-pull otherwise.
- Docs: `website/docs/stacks/imports.mdx` and `website/docs/cli/configuration/imports.mdx`.

## Validation

- `pkg/stack/imports/remote_oci_test.go` was written first and failed with the reported error before the fix.
  It passes now, using the in-process registry from `pkg/oci/ocitest`.
- Coverage added: file, extensionless file, directory, and glob selection; missing path, path traversal, path
  absent from the artifact, and unreachable registry; XDG cache location; one pull per run; cross-run reuse with
  a ttl and re-pull without one; `nested_imports: remote`; `IsOCIURI`.
- `pkg/config/adapters/adapters_test.go`: an `atmos.yaml` import of an OCI file with a nested OCI import, and the
  missing-path error.
- `internal/exec/stack_processor_utils_test.go`: a stack manifest importing an OCI artifact with
  `nested_imports: remote`, end to end.
- `go build ./...` and `go test ./pkg/stack/... ./pkg/config/... ./pkg/downloader/... ./errors/...` pass.
- `atmos lint --changed` reports 0 issues.
- Also covered: a hidden directory import (`//.atmos.d`) at both the importer and `atmos.yaml` adapter levels
  (this exposed the extension defect above), cached files keeping `.yaml` and `.yml` extensions, new content
  being served after a re-pull with no ttl, and cached content being kept while a ttl is fresh.
- `TestRemoteImporter_Resolve_GitSubdir_RefreshesCachedFiles` covers the wider effect of the `cacheFile` change on
  Git subdirectory imports. Against the original `cacheFile` it fails (a new commit is not picked up: `v1` is served
  instead of `v2`), so the stale-copy defect already existed for Git imports. With the fix it passes, and a fresh
  `ttl` still keeps the cached content.
- A content-addressing test (a new revision gets a new path and the previous revision stays intact) and unit tests
  for the GHCR environment fallback (precedence, nil configuration, no mutation of the original, and a token without
  a username staying anonymous). A throwaway probe confirmed that `settings` is empty during `atmos.yaml` import
  processing even when `GITHUB_TOKEN` and `GITHUB_ACTOR` are set.
- `pkg/cache` extension cases for fragments, including `uri?ref=v1#path/to/file.yaml`.
- Not run: `atmos test --full`, and an import against a real registry (GHCR or ECR). Registry behavior was
  covered only through the in-process test registry.

## Follow-ups

None.
