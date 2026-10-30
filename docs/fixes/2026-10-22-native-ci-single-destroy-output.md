# Fix: Emit native CI results for single-component Terraform destroy

**Date:** 2026-10-22

## Summary

A single-component Terraform destroy emits configured CI outputs, job summaries and check updates, matching other Terraform mutations.

## Context

An actual local OpenTofu apply/destroy under a simulated GitHub runner produced plan/apply outputs and summaries, but successful single destroy produced neither. This repeated twice. The Terraform CI plugin registered aggregate destroy hooks only, and had no individual destroy summary template.

## Changes

- Capture destroy output and emit before/after lifecycle events, including failures.
- Bind individual destroy events to the shared mutation CI handlers and wire per-component user hooks without duplicating aggregate CI results.
- Add a destroy summary template for successful, unchanged and failed results.
- Preserve existing output allowlists and independent summary/check/output settings.

## Validation

A regression test failed because the destroy hook was absent. Tests now invoke the registered hook and assert summary content, command/component outputs and destroyed resource counts, including error summaries. The full Terraform CI plugin suite remains covered. Runtime verification now confirms a successful one-resource destroy writes both native outputs and a summary containing its destroyed-resource count. It uses a disposable local resource and writes only local GitHub output/summary files; no external check or comment is posted.

## Follow-ups

None.
