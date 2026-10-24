# Fix: Validate the Kubernetes API port before emulator startup

**Date:** 2026-10-24

## Summary

An emulator port override missing the required Kubernetes API mapping now fails during spec validation.

## Context

On main at `cb23b3f701`, replacing the k3s port list with only container port 80 passed resolution and later failed with `no bound API server port`.

## Changes

Require container port `6443/tcp` for Kubernetes-target emulator specifications. Preserve existing port-list replacement semantics and explain how to retain the API mapping in user and agent documentation. Tests exercise spec validation without starting Docker.

Compare the full returned port mappings for defaults and valid overrides, including host ports and explicit protocols, so equal-length but incorrect mappings fail the regression tests.

## Validation

The regression failed before the fix. Ran `go test ./pkg/emulator/... ./pkg/component/emulator -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the combined website build passed. No Docker or cloud access was used.

## Follow-ups

None.
