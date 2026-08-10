# Fix: native Kubernetes commands expose --ci

**Date:** 2026-07-02

## Problem

Native Kubernetes lifecycle execution already passed `viper.GetBool("ci")` to
CI hooks, but the Kubernetes operation commands did not register a `--ci` flag.

As a result, automatic CI detection could still work, but users could not force
generic/local CI hook behavior for Kubernetes commands the way they can for
Terraform, Helmfile, and native Helm.

## Fix

All native Kubernetes operation commands now register `--ci`.

The flag is available alongside the existing selection flags (`--all`,
`--affected`, `--include-dependents`) and feeds the existing Kubernetes CI hook
path without changing automatic CI detection.

## Tests

```shell
go test ./cmd/kubernetes -count=1
```
