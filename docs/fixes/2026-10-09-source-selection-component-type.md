# Fix: Select source components and stacks by command type

**Date:** 2026-10-09

## Summary

Source component and stack selectors now use the component type configured by their command builder.

## Context

The shared source selectors always read `components.terraform`. CloudFormation-only projects therefore had no selectable components, and mixed projects could offer Terraform components or stacks for CloudFormation commands. Helmfile and Packer commands shared the same defect.

## Changes

- Bind each source command to its configured component type using command metadata.
- Thread that type through candidate collection and stack filtering, preserving sorted, unique component names and source-only selection.
- Return no candidates for commands without a configured type, rather than guessing from display names or the environment.
- Add regressions for CloudFormation-only projects, all four command builders and provider types, same-name components across providers, and stack filtering.

## Validation

- New selector regressions failed before the fix: CloudFormation returned Terraform candidates and stacks, and CloudFormation-only components were absent.
- `go test -race ./pkg/provisioner/source/cmd ./cmd/aws/cloudformation/source ./cmd/terraform/source ./cmd/helmfile/source ./cmd/packer/source -count=1` passed.

- `go build ./...` passed.
- Patch-scoped custom GolangCI lint reported zero issues.
- Fix-log validation and `git diff --check` passed.

## Follow-ups

None.
