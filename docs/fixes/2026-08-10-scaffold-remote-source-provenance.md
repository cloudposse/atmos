# Fix: remote scaffold sources no longer record a dangling temp-dir path in `spec.source`

**Date:** 2026-08-10

## Summary

`atmos scaffold generate`/`atmos init` recorded a dangling, already-deleted temp-directory path in
`spec.source` of `.atmos/scaffold.yaml` whenever the template source was remote (`git::...` or a bare
`https://...` URL). `resolveRemote()` in `pkg/generator/source/resolver.go` now restores
`Configuration.Source` to the original source string the caller passed in after loading the template
config from the temporary download directory.

## Context

- For a remote scaffold source, `resolveRemote()` downloads the template into
  `os.MkdirTemp("", "atmos-scaffold-")`, then loads the config with that temp dir passed in as the
  "source." `Configuration.Source` — and therefore the persisted `spec.source` — ended up holding
  something like `/var/folders/xx/.../atmos-scaffold-1234567890`. `cleanup()` removes that directory
  immediately after the command finishes, so the recorded provenance was a dangling reference to nothing
  as soon as generation completed, contradicting `SaveProjectRecord`'s own doc comment: "spec.source and
  spec.baseRef record provenance for future updates."
- Local sources (a relative/absolute path, or `file://...`) were already correct, but only by accident of
  `resolveLocal` having no temp-dir indirection step, not because anything special-cased provenance for
  them.
- Reproduced directly: `atmos scaffold generate "git::https://.../scaffold-template.git" ./out --defaults`,
  then `cat ./out/.atmos/scaffold.yaml` showed a `/var/folders/...`/`/tmp/...` path for `spec.source` that
  no longer existed on disk.
- No upstream GitHub issue; PR https://github.com/cloudposse/atmos/pull/2869 was opened directly.

## Changes

- `pkg/generator/source/resolver.go`: in `resolveRemote()`, set `conf.Source = src` (the original source
  string the caller passed in) after loading the template configuration from the temporary download
  directory, instead of leaving `Configuration.Source` as whatever `LoadConfigurationFromDir` was given
  (the temp dir itself). No change to `resolveLocal`, which already recorded the correct value.
- `pkg/generator/source/resolver_test.go`: added assertions that `Configuration.Source` holds the original
  source for both local (`TestResolve_LocalPath`, `TestHydrate_LocalStub`) and remote paths, plus a
  dedicated regression test, `TestResolve_RemoteRecordsOriginalSource`, that fails on the pre-fix code and
  passes after. The remote case is exercised by serving an in-memory ZIP archive from an `httptest.Server`
  rather than a git fixture — go-getter's HTTP getter unpacks the `.zip` extension on its own, so the test
  never shells out to git or any other external binary and stays hermetic/cross-platform.

## Validation

- `go test ./pkg/generator/source/...` — passes, including `TestResolve_RemoteRecordsOriginalSource`.
- Manual repro: `atmos scaffold generate "git::https://.../scaffold-template.git" ./out --defaults` followed
  by `cat ./out/.atmos/scaffold.yaml` now shows the original `git::...` source string in `spec.source`
  instead of the removed temp directory.

## Follow-ups

None.
