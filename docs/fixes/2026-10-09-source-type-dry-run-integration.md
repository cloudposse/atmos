# Fix: Preserve provider selection through source dry-run validation

**Date:** 2026-10-09

## Summary

The shared source loader and dry-run paths retain the command's provider when resolving same-name components.

## Context

The generation and source-provisioning layer refactors source deletion into `loadSourceComponent` and adds validation to dry runs. These helpers must preserve the provider-specific configuration lookup introduced below that layer.

## Changes

- Pass the component type through `loadSourceComponent`, `sourceConfigInfo`, dry-run pull, and deletion ownership configuration loading.
- Keep target resolution, ownership checks, provenance requirements, confirmation behavior, and dry-run side-effect suppression intact.
- Update the shared dry-run test seam to require CloudFormation's component type.
- Supply provenance in the mixed-provider deletion regression and exercise real mixed-provider dry runs for pull and delete.

## Validation

- `go test -race ./pkg/provisioner/source/... ./cmd/aws/cloudformation/source ./cmd/terraform/source ./cmd/helmfile/source ./cmd/packer/source -count=1` passed.
- `go build ./...` passed.
- `atmos lint --changed` passed with zero issues.
- Shared source command statement coverage was 87.7%; provider selection helpers, `loadSourceComponent`, and `sourceConfigInfo` each had 100% statement coverage.

## Follow-ups

None.
