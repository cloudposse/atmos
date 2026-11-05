# Fix: Reuse logger output identities and resolve relative process paths once

**Date:** 2026-10-04

## Summary

Repeated logger setup reuses Charm renderer keys for the same destination. Processes launched with a relative working directory and relative PATH entries resolve the executable correctly.

## Context

CodeRabbit identified two issues in the execution support layer. Each file output acquired a fresh pointer wrapper, which Charm retained as a distinct renderer registry key. Invocation PATH lookup returned a relative executable path including the working directory, which `exec.Cmd` then interpreted relative to that directory a second time.

## Changes

The opaque logger writer now uses a comparable value when its destination supports equality. Repeated setup therefore produces the same renderer registry key without adding another cache. The stderr value resolves the current `os.Stderr` lazily. Non-comparable custom file-like writers retain their existing pointer wrapper behavior, preserving compatibility with Charm's map keys.

Process lookup resolves the invocation directory to an absolute path before searching its environment. The subprocess retains its requested working directory and original command arguments. Regression tests cover relative PATH entries, relative explicit executable paths, and an empty directory, using a copied Go test executable on all platforms.

## Validation

Both regressions failed before the fixes: repeated writer setup produced 20 distinct keys, and process startup attempted an executable path relative to the working directory twice. Tests also verify independent logger destinations after output changes and custom non-comparable writers.

- `go test -race -count=3 -timeout=10m -coverprofile=/tmp/atmos-support-review.cover ./pkg/logger ./pkg/process` passed; logger coverage was 96.5% and process coverage was 94.8%.
- The final package coverage run (`go test -count=1 -timeout=10m -coverprofile=.context/support-review.cover ./pkg/logger ./pkg/process`) passed with the same package coverage. Combined with the other changed-package tests, local changed-line coverage against `origin/main` was 585/615 (95.12%). This touched-package estimate is not full-suite coverage; CI Codecov remains authoritative.
- `go build ./...` passed.
- `./custom-gcl run --new-from-rev=HEAD ./pkg/logger/... ./pkg/process/...` passed with zero issues.

## Follow-ups

None.
