# Fix: Replace nonexistent public OCI component examples

**Date:** 2026-10-22

## Summary

Vendoring documentation uses a public image example and generic repository placeholders, and explains that any OCI image can provide source files.

## Context

The published `public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest` example returned `NAME_UNKNOWN` on two anonymous pulls: the repository does not exist. This was a broken example, not a failure of OCI transport.

## Changes

The vendoring skill uses `oci://docker.io/library/nginx:alpine` as a public example. Website vendor configuration, imports, component manifests, commands, and cheatsheets use generic image references. OCI images do not require an Atmos-specific format, repository layout, or publisher. Select an existing image containing the files needed at the target.

## Validation

Executed `atmos vendor pull` twice against the original ECR URI in an isolated Git fixture with empty Docker credential configuration; both returned `NAME_UNKNOWN`. Executed the same command against `oci://docker.io/library/nginx:alpine`; it succeeded and materialized the nginx binary. Evidence: `.context/skills-field-test/yaml-public-oci-results.json` and `yaml-public-oci-control-results.json`. The scripts with those names minus `-results` recreate the fixture and invocation. These pulls do not run the container or deploy resources.

## Follow-ups

None.
