# Fix: hash module and exec.which in the Automation Language

**Date:** 2026-10-07

## Summary

Scripts can now compute digests with `hash.sha256`, `hash.sha512`, `hash.sha1`, and `hash.md5`, and can ask `exec.which(name)` which executable `exec.run` would start. Both close gaps found while mapping a Python pipeline harness onto the automation language.

## Context

A Python test harness for a CloudFormation example used `hashlib.sha256` to fingerprint built archives and `shutil.which` to check prerequisites. The automation language had no digest function at all, and the only way to learn whether a tool existed was to run it and catch the start failure. The process runner already resolved commands against the script's own `PATH` through `interp.LookPathDir`, but that logic was private to `pkg/process`.

## Changes

- Added `process.LookPath(dir, env, name)` in `pkg/process/lookpath.go` and made `resolveCommand` use it, so `exec.run` and `exec.which` share one resolution rule.
- Added the stateless `hash` module in `pkg/script/starlark/stdlib/hash`. Each function takes one string and returns lowercase hex. Starlark strings are byte strings, so `hash.sha256(fs.read_file(path))` hashes binary files exactly. `md5` and `sha1` carry `gosec` suppressions with a stated interoperability purpose.
- Added `exec.which(name)` in `pkg/script/starlark/process_which.go`. It searches the effective execution environment, including directories added by `dependencies.tools`, honors cancellation, rejects an empty name, and returns `None` when nothing matches.
- Documented five functions, listed them in the function index, the built-in reference, the language overview, the Python differences table, the script step table, the agent skill, and the Starlark PRD mapping table.

## Validation

- `go build ./...`
- `go test ./pkg/process/ ./pkg/script/starlark/... -count=1`
- `./custom-gcl run --new-from-rev=origin/main ./pkg/process/ ./pkg/script/starlark/...`
- New tests cover known-answer digests, non-ASCII input, binary file contents through `fs.read_file`, PATH-scoped lookup using the test binary (cross-platform), `None` for a missing tool, and argument validation.
- `cd website && npm run build`

## Follow-ups

None.
