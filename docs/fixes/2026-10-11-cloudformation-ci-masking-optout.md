# Fix: Document the CI output masking opt-out

**Date:** 2026-10-11

## Summary

Document the CI output masking opt-out.

## Context

The CI guide promised NoEcho-dependent output values were never exported in clear text.

## Changes

Explain that presentation masking applies by default and --mask=false can expose those values.

## Validation

Checked the masking flag, CloudFormation output masking, and CI export path against the corrected wording.

## Follow-ups

None.
