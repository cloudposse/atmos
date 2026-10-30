# Fix: Accept schema directory overrides during component validation

**Date:** 2026-10-22

## Summary

`atmos validate component` accepts the documented `--schemas-jsonschema-dir` and `--schemas-opa-dir` overrides and uses them to select validation files.

## Context

The raw argument parser and configuration loader already supported both flags. The Cobra-based validation command did not register them, and its parsed flags were missing from the configuration handoff.

## Changes

Register both flags with the standard parser. Preserve explicitly supplied Cobra flag values when creating the configuration-loading context. Keep the existing raw-argument path and config/environment precedence. Reconcile the command, configuration, validation, and skill documentation with the working interface.

## Validation

The CLI regression executes actual JSON Schema and OPA validation with intentionally nonexistent configured and environment-provided bases. Explicit flags select valid policy directories and pass. Selecting alternate rejecting JSON or OPA directories fails for the corresponding policy. It covers both equals and separated flag forms, an absolute directory, and a directory relative to the project base.

## Follow-ups

None.
