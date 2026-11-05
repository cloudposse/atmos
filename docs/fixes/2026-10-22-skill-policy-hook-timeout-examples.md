# Fix: Correct field-tested policy, hook, and stop examples

**Date:** 2026-10-22

## Summary

OPA policy examples import the membership keyword they use. Command hooks separate the executable from its arguments. Devcontainer stop examples use integer seconds.

## Context

The field test reproduced policy parse failures, command-hook dispatch failures, and rejected duration-suffixed stop timeouts. Successful controls established the supported syntax without changing runtime behavior.

## Changes

Add `import future.keywords.in` to four membership-using policy snippets. Update Kubernetes notification hooks in the skill and website to `kind: command`, `command: echo`, and `args`. Update both TFLint hook examples to `command: tflint` with `args: [--init]`; hooks already run in the component directory. Document `--timeout 30` as whole seconds.

## Validation

The field test ran the three tag/Kubernetes-version/cost OPA policy controls through component validation after adding the import, all successfully. The fourth command-aware policy receives the same required import but its environment-dependent behavior was not separately executed. Kubernetes delivery hooks executed twice using the corrected structure. TFLint initialized and Terraform planned successfully with the separated arguments. Devcontainer stopped and removed its disposable container with an integer timeout; duration-suffixed inputs failed twice. The field-test report retains baseline failures and control commands.

## Follow-ups

Complete command-aware policy environment/argument behavior remains a field-test coverage gap. Unsupported schema `!import` and other separately recorded findings remain in the field-test report.
