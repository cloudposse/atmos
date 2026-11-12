# Fix: Gitea fork and private repository checkout

**Date:** 2026-11-11

## Summary

Correct Gitea Actions checkout for fork pull requests and private repositories.

## Context

Field testing found that fork-only branches were requested from the base repository and that the workflow guides omitted Git credentials.

## Changes

Ordinary pull request contexts select the validated fork head repository; elevated events retain the base checkout and safety gate. Workflow checkout steps provide the job token through a process-scoped Git credential helper.

## Validation

`go test ./pkg/ci/providers/gitea ./cmd/git` passed, including fork and elevated-event context coverage. `git diff --check` passed. Against a disposable Gitea 28.3 server, the updated binary cloned a fork-only PR branch from the fork and cloned a private repository using a read-scoped token and the documented helper.

## Follow-ups

PR #3661 separately adds exact-ref checkout for event revision pinning.
