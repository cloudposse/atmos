# Fix: Resolve declared TFLint dependencies before rejecting availability

**Date:** 2026-10-22

## Summary

Standalone Terraform lint accepts TFLint declared in merged stack/component dependencies even when it is absent from the ambient PATH and `.tool-versions`.

## Context

The early availability check rejected the command before loading component dependencies. Its own error advised adding a declaration that this check never reached. Project tool paths also preceded explicit component versions.

## Changes

Resolve project pins opportunistically, then defer missing-tool diagnostics to component execution after dependency resolution. Component tool paths take precedence over the project fallback.

## Validation

An integration regression executes a local scanner fixture with an empty PATH and only a component dependency declaration. Existing scanner tests pass. A live CLI test installed TFLint into a fresh disposable tool directory, linted real HCL successfully, and repeated successfully. Invalid HCL still returned a nonzero result with the actual TFLint finding.

## Follow-ups

Projects without any usable TFLint declaration or executable still fail at the per-component scanner check.
