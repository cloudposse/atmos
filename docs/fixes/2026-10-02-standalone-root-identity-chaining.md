# Fix: Chaining from a standalone identity fails with "provider not registered"

**Date:** 2026-10-02

## Summary

An identity that chains from a standalone identity (`via: identity: <standalone>`) failed to authenticate
whenever the standalone root had no valid cached credentials. The auth manager tried to authenticate the root
as a *provider* and failed with `provider "<name>" not registered: invalid auth config`. The manager now
authenticates a standalone identity at the root of a chain through the `StandaloneIdentity` interface.

## Context

Chaining from a standalone identity builds the chain `[root-identity, child, ...]`. When no cached credentials
were found anywhere in the chain, `authenticateProviderChain` in `pkg/auth/manager_chain.go` always called
`authenticateWithProvider(m.chain[0])`, even though `m.chain[0]` is an identity, not a registered provider.

The bug was latent:

- `aws/ambient` roots hid it because their `LoadCredentials` re-resolves credentials live, so the root always
  looked cached.
- `aws/user` roots usually hid it because the long-lived keys in the keyring count as non-expiring cached
  credentials. An `aws/user` root configured with YAML keys (no keyring entry) hit it as soon as its session
  files expired.
- The new `aws/credential-process` identity kind (#3248, #1734) would always hit it, because its
  `LoadCredentials` only reads Atmos-managed session files and must stay side-effect free.

## Changes

- `pkg/auth/manager_chain.go`: extracted root authentication into `authenticateChainRoot`. A registered
  provider takes the unchanged path (`PreAuthenticate`, then `authenticateWithProvider`). A root that is not a
  provider but implements `types.StandaloneIdentity` with `IsStandalone() == true` is authenticated by
  `authenticateStandaloneRoot`, which calls `AuthenticateStandalone`. Its credentials are not written to the
  keyring (the standalone identity owns its storage). Anything else still reports "provider not registered".
- `pkg/auth/manager_chain_standalone_root_test.go`: repro and negative tests.

## Validation

- Wrote the repro test first; before the fix it failed with
  `provider "root" not registered: invalid auth config`. It passes after the fix.
- Negative tests: a chain rooted at a registered provider never calls the standalone path; an unregistered,
  non-standalone root still fails with "not registered"; a failing standalone root fails the chain without
  running the child.
- `go test -short ./pkg/auth/... ./cmd/auth/... ./cmd/aws/...` passes.
- End-to-end against the Floci AWS emulator: `TestAWSCredentialProcessFlociE2E_AssumeRoleChain` authenticates
  an `aws/assume-role` chained from an `aws/credential-process` root.

## Follow-ups

None.
