# Fix: Validate auth provider examples

**Date:** 2026-10-22

## Summary

Correct GCP provider, GitHub OIDC and AWS account configuration examples so their complete YAML passes strict auth validation.

## Context

The field test reproduced missing GCP provider specs, a missing GitHub OIDC audience, and scalar AWS account selectors. These were example errors; validation correctly rejected the incomplete configuration.

## Changes

- Put GCP ADC and workload identity federation provider fields under `spec` in the auth skill, provider reference and GCP migration reference.
- Include the required GitHub OIDC `spec.audience` in the complete chain example and correct comments that called it optional.
- Express ECR/EKS permission-set account selectors as `account: {name: dev}`.
- Reconcile matching website command/configuration/tutorial examples, including production account selectors and the GitHub Actions role identity fields. Correct the GCP service-account principal key in the multi-cloud overview.
- Document destroy CI summaries/template overrides, preserved saved plans, and deferred executable secret selectors. Existing SOPS keygen documentation already matches the restored first-write behavior.

## Validation

Extracted all ten corrected complete YAML examples directly from their edited text and ran `atmos --config=/absolute/fixture/atmos.yaml auth validate` twice per example. All twenty invocations passed. The original examples failed on the unchanged baseline; the audit ledger retains those observations and corrected examples.

Static validation establishes configuration shape and chain consistency. Real GCP federation, GitHub token exchange, ECR login and EKS acceptance still require their external account prerequisites.

## Follow-ups

Keep live cloud authentication separate from static example validation in the field-test coverage ledger.
