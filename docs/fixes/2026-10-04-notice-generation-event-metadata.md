# Fix: Generate NOTICE using pull-request event metadata

**Date:** 2026-10-04

## Summary

Pull-request CI passes the repository description from its event to NOTICE
generation, avoiding an unnecessary anonymous GitHub API request.

## Context

PR #3261's autofix job failed while generating NOTICE: the repository metadata
endpoint returned HTTP 403. The generator already supports authenticated local
requests, but PR-controlled Go code must not receive a CI authentication token.
The public repository description is already present in the pull-request event.

## Changes

- Both NOTICE-generation workflows pass `ATMOS_NOTICE_REPO_DESCRIPTION` from the
  event through a step environment variable, without shell interpolation.
- The generator validates supplied metadata and makes no network request when it
  is present. Empty, multiline, or non-printable descriptions fail explicitly.
- Local invocations without the override retain the existing GitHub API behavior.
- No token is added to the Go execution steps.

## Validation

- `go test -C tools/noticegen -race ./...` passed with 96.0% statement coverage.
- Regression cases use an invalid URL to prove valid and invalid event metadata
  never enter the network path.
- Existing HTTP response, authentication, and description validation tests pass.
- `go tool mage notice:generate` passed with the public repository description
  supplied through the new environment variable; the generated NOTICE was unchanged.

## Follow-ups

None.
