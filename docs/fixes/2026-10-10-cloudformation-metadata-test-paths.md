# Fix: CloudFormation metadata test fixtures use template paths

**Date:** 2026-10-10

## Summary

Keep inherited metadata regression fixtures valid when CloudFormation introduces inline templates.

## Context

Earlier stack layers use `template` for a filename. This layer introduces `path` for files and interprets `template` as inline template content. The inherited metadata tests must follow that contract to reach the operation guards they exercise.

## Changes

Use `path: template.yaml` in the component, changeset, and StackSet metadata test fixtures. Preserve all lock, abstract-component, dry-run, and preview assertions.

## Validation

`GOTOOLCHAIN=go1.26.9 go test -race ./pkg/component/aws/cloudformation -count=1` passed on this owning layer (11.568 seconds). The fix-document validator passed. Combined-stack validation follows the cascade.

## Follow-ups

None.
