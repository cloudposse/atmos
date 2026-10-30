# Fix: Match auth examples to their credential prerequisites

**Date:** 2026-10-24

## Summary

AWS OIDC examples use the audience in the linked GitHub trust policy, and GCP ADC documentation limits interactive login instructions to local user credentials.

## Context

A regional AWS audience did not match the linked trust-policy example. Unqualified gcloud login instructions incorrectly applied to service-account key files and attached service accounts.

## Changes

Use `sts.amazonaws.com` consistently in this PR's GitHub OIDC examples. Explain the ADC credential source distinctions in the provider page and skill reference, including how to switch file-based credentials. Point the CI authentication introduction to `profiles/github/auth.yaml`, matching the profile example and `ATMOS_PROFILE: github` selection.

## Validation

Checked the examples against the official [GitHub AWS OIDC trust-policy guide](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws) and [Google ADC search-order documentation](https://docs.cloud.google.com/docs/authentication/application-default-credentials). These are documentation changes; no live-account authentication was performed.

## Follow-ups

None.
