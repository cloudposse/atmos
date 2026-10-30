# Fix: Clarify deployment selection and custom-command directory scope

**Date:** 2026-10-23

## Summary

Skills and command documentation distinguish CloudFormation bulk selection from cross-type dependency declarations, and custom-command step paths from command-level paths.

## Context

The CloudFormation skill implied that bulk commands deploy Terraform prerequisites. The hooks skill implied that custom-command step overrides resolve against the project base path. Both claims could cause users to run operations with unmet prerequisites or in the wrong directory.

## Changes

Explain that CloudFormation bulk commands select only CloudFormation components, and direct users to deploy Terraform prerequisites separately. Document command-level directory resolution against `base_path`, step overrides against the resolved command directory, and the invocation-directory fallback. Reconcile the corresponding website pages and custom-command reference. Add descriptive comments to the published-example test helpers flagged by review.

## Validation

Focused tests pass for published skill examples, directory resolution, CloudFormation bulk selection, and stack `!cwd`/`!random` evaluation. Three disposable CLI probes confirm command-relative overrides, invocation-relative overrides, and directory inheritance. The existing binary was reused because these changes affect only documentation and Go comments.

## Follow-ups

None.
