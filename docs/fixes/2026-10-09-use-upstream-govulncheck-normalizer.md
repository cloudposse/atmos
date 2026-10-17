# Fix: Use the upstream govulncheck SARIF normalizer

**Date:** 2026-10-09

## Summary

The CloudFormation stack uses the `ci:vulncheck` Mage target merged into main in PR #3347.

## Context

The stack carried a Python normalizer for duplicate SARIF stacks. Main now provides the same protection through a tested Go package and an atomic report writer, along with Go and dependency security updates. Both changes touched the scan step and conflicted when rebasing.

## Changes

- Preserve main's Mage scan step, security updates, and exact artifact action pin.
- Remove the unused Python normalizer and its duplicate tests.
- Retain the stack's required network endpoints for the scan bootstrap.

## Validation

- `go test -race ./internal/ci/vulncheck -coverprofile=... -count=1` passed, with 90.8% package statement coverage.
- The upstream suite covers duplicate and distinct stacks, all findings, empty results, malformed input, failed scans, and preservation of existing reports.
- The scan workflow differs from main only by the two required bootstrap endpoints.

## Follow-ups

None.
