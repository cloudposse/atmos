# Fix: Run secret selector execution tests on Windows

**Date:** 2026-10-26

## Summary

Secret selector and dependency execution tests use a Go subprocess helper instead
of `touch`, so Windows no longer skips their assertions.

## Context

The two regression tests skipped Windows because their side-effect markers relied
on a Unix command. This left Windows without coverage for deferred selector
execution, dependency evaluation, and edition-dependent quoted function behavior.

## Changes

- Invoke the current test executable with an exact helper sentinel; `TestMain`
  writes the requested marker with `os.WriteFile` and exits with empty stdout.
- Pass executable and marker paths through double-quoted environment expansions,
  preserving spaces, apostrophes, and Windows backslashes without interpolating
  paths into YAML or template source.
- Remove both Windows skips while retaining native/current-quoted/legacy-quoted
  execution cases, exact selector error checks, untouched sibling declarations,
  and no-side-effect checks for discovery and service loading.
- Exercise marker paths containing spaces and apostrophes. Production code is
  unchanged.

## Validation

`go test -race ./cmd/secret -run 'TestSecretDiscoveryDefers' -shuffle=on -count=2`
passed on macOS, executing the subprocess helpers through actual selector and
dependency evaluation. `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c
./cmd/secret` also passed. Cross-compilation does not verify Windows execution;
Windows runtime verification remains pending CI. Scoped custom lint
(`custom-gcl run --config=.golangci.yml --new-from-rev=origin/main ./cmd/secret/...`)
reported zero issues. The fix-record validator and whitespace check passed.

## Follow-ups

Run the enabled tests in Windows CI to verify runtime behavior on that platform.
