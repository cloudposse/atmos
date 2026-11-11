# Fix: Run every secret command on stack-scoped secrets without a component

**Date:** 2026-11-03

## Summary

`atmos secret get`, `delete` (`unset`), `import`, `pull`, `push`, `exec`, and `shell` accept `--stack` alone for stack-scoped and global secrets, as `set` and `validate` already did.

## Context

#3517 made `--component` optional for `set` and `validate` only. `atmos secret unset NAME -s organization` (a stack-level SOPS secret) and `atmos secret shell -s organization` still prompted for, or failed without, a component.

## Changes

Generalize the shared-declaration resolution used by `set` into `cmd/secret/shared_scope.go`. Named commands (`get`, `delete`, `import NAME --from-*`, `set`) require one consistent shared declaration. Whole-scope commands (`pull`, `push`, `import FILE`, `exec`, `shell`, `delete --all`) operate on the stack's shared declarations only, through a service restricted to them, so instance secrets are never read or written without `--component`. When no shared context resolves, a TTY prompts for a component and everything else gets the required-flag error naming the reason. Success and status messages name the stack instead of an arbitrary loading component.

## Validation

Table tests cover every command componentless, explicit `--component` keeping instance scope, and the still-required cases (instance secret, differing declarations or providers, no shared secrets, enumeration failure). The offline SOPS round trip covers `set`, `get`, `pull`, `delete`, and `delete --all` without a component and checks an instance secret is untouched. Ran `go build ./...` and `go test ./cmd/secret ./pkg/secrets -count=1`, which pass, and the fix-doc validator. `golangci-lint` was blocked by the session's permissions and the website build was not run, so both remain to be run before the PR is marked ready.

## Follow-ups

None.
