# Fix: Restrict service container diagnostics

**Date:** 2026-10-14

## Summary

Keep service recovery diagnostics without exposing inspected container credentials.

## Context

Full Docker inspection includes environment variables, while health-check output can also contain
secrets generated during a job. Failure logging must preserve the original startup error.

## Changes

Print only status, running state, exit code, and health status from inspection. Keep bounded service
logs and isolate diagnostic failures. Clarify the CloudFormation PRD limitation for templates that
combine macros with nested stacks, which AWS requires deploying through direct stack operations.

## Validation

All ten service-restoration unit tests pass, including credential exclusion, malformed inspection,
health polling, and failure diagnostics. Checked the macro limitation against the AWS CreateChangeSet
API reference. Workflow security policy and service-health requirements remain intact.

## Follow-ups

Verify final published CI and CodeRabbit review in the active stack.
