# Fix: Reconcile skill descriptions with execution contracts

**Date:** 2026-10-22

## Summary

Correct recording format, verbosity default, tool installation policy, directory resolution, and project initialization descriptions.

## Context

The broader field test found prose claims that disagreed with actual executions or configuration defaults, independently of the four original reports.

## Changes

Describe v2/v3 cast support and v3 recording, the false custom-command verbosity default, and the `toolchain.install` policy used by subsequent commands. Explain workflow and custom-command base paths separately from extended-step and component hook paths. Correct the init skill and command page: Git initialization skips destinations within an existing repository.

## Validation

Inspected generated cast headers and playback deltas. Executed commands and workflow steps with distinct invocation and base directories, plus component hooks with eight directory shapes. Ran init with default Git behavior, explicit `--git`, and `--no-git` in disposable standalone targets, and with a target inside an existing repository; asserted repository creation and commit presence. Repeated the custom-command verbosity discrepancy on the baseline and current binaries.

## Follow-ups

The pre-existing custom-command `verbose: true` command-echo discrepancy remains recorded in the field-test report. Correcting its documented default does not resolve that runtime defect.
