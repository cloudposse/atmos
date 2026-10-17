# Fix: Isolate Git fixtures with a Windows-compatible null config

**Date:** 2026-10-15

## Summary

Use Git's `/dev/null` configuration sentinel on all platforms so Windows acceptance test startup can build its local Git mirror.

## Context

The Windows 2022 build using Git 2.56.0.windows.1 failed before test enumeration because Git rejected `GIT_CONFIG_GLOBAL=NUL` with `Invalid argument`. Go's `os.DevNull` returns `NUL` on Windows, but Git documents `/dev/null` for disabling global or system configuration reads.

[Git for Windows issue #6449](https://github.com/git-for-windows/git/issues/6449) confirms this regression; 2.56.0.windows.2 fixes it. Using `/dev/null` also supports runner images that still ship the affected release. The Windows diagnostic pins the affected Git version because hosted runners can receive either image during rollout.

## Changes

- Use `/dev/null` in the mirror builder and the Git provider, provisioner, scaffold, and local GitOps test helpers.
- Preserve host configuration isolation and the existing system-config and command-config filtering.
- Add a regression that proves malformed ambient configuration breaks an ordinary Git command while fixture construction still succeeds.

## Validation

- Go 1.26.9 race tests passed for the Git mirror, CLI Git provider, and Git provisioner packages.
- `go build ./...` and acceptance-suite startup (`go test ./tests -run '^$'`) passed locally.
- Patch-scoped `atmos lint --changed` reported zero issues; focused `atmos test` passed.
- Windows validation and the final stack CI results are recorded with the PR checks before completion.

## Follow-ups

Require the focused Windows diagnostic and final published-head CI checks to pass before considering the stack complete.
