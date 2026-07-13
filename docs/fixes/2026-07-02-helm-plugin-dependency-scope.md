# Fix: native Helm plugin commands use Helm dependency scope

**Date:** 2026-07-02

## Problem

Native Helm plugin commands resolved the `helm` binary using the `helmfile`
component dependency scope.

That meant `components.helm.dependencies` was ignored for commands such as
`atmos helm plugin list` and `atmos helm plugin install`, while
`components.helmfile.dependencies` could incorrectly influence native Helm
plugin behavior.

## Fix

Helm plugin binary resolution now asks the dependency resolver for the native
`helm` component type.

This keeps native Helm plugin management aligned with native Helm component
execution and avoids cross-talk with Helmfile dependency configuration.

## Tests

```shell
go test ./cmd/helm -run TestResolveHelmBinary -count=1
```
