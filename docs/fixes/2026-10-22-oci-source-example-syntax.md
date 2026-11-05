# Fix: Use the supported OCI source URI syntax

**Date:** 2026-10-22

## Summary

Component source examples use `oci://registry/repository:tag`, which reaches the OCI downloader.

## Context

Helmfile and Packer skills and component source command documentation used `oci::registry/repository:tag`. A disposable local OCI registry reproduced parsing failure with that spelling twice. The equivalent `oci://` URI downloaded and extracted the expected Helmfile fixture successfully.

## Changes

Correct the URI scheme in Helmfile and Packer skills and references and the Helmfile, Packer, Terraform, and CloudFormation source command documentation. Preserve the documented OCI capability and version pinning.

## Validation

Publish a minimal image containing `helmfile.yaml` and `values.yaml` to a local registry. Invoke `atmos helmfile source pull` with each spelling. The corrected spelling produces the exact `values.yaml` contents; the previous spelling fails before download. Stop the disposable registry afterward.

## Follow-ups

None.
