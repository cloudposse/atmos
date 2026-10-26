# Fix: Isolate Terraform init hook test selection state

**Date:** 2026-10-25

## Summary

Terraform init hook tests restore the component selection and multi-component marker after each test, so shuffled runs do not affect later hook-context tests.

## Context

After the prepared-workdir lifecycle changes from #3462, `initCmd.PreRunE` also records `preResolvedComponent`. `TestInitCmd_HookWiring` selected `myapp` without restoring that package-level value. A later multi-component hook-context test inherited `myapp` and failed its empty-component assertion. This surfaced in PR #3526's race suite after merging main; it was test-state contamination, not a data-race report.

## Changes

Save, clear, and restore both selection globals in the two init hook tests. Keep production behavior unchanged. The existing init hook and multi-component hook-context tests reproduce and guard the failure under the reported shuffle seed, so no redundant regression test is added.

## Validation

The two-test reproduction failed before the fix with shuffle seed `1792957269469291470`: expected an empty component, got `myapp`. After the fix, those tests plus the init error-hook test passed with `-race` and the same seed (4.452s). The full `cmd/terraform` short-mode suite passed with `-race`, the same shuffle seed, and a ten-minute timeout (273.059s). `go build ./cmd/terraform` passed. Patch-scoped custom lint reported zero issues; fix-log validation and `git diff --check` passed.

## Follow-ups

None.
