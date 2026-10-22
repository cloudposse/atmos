# Fix: Replace nonexistent public OCI component examples

**Date:** 2026-10-22

## Summary

Vendoring documentation now distinguishes a verified public container image from project-owned Terraform component images.

## Context

The published `public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest` example returned `NAME_UNKNOWN` on two anonymous pulls: the repository does not exist. This was a broken example, not a failure of OCI transport.

## Changes

The vendoring skill uses `oci://docker.io/library/nginx:alpine` as its public example and explains that it extracts a container filesystem into an asset directory, not Terraform component files. Website vendor configuration, imports, URL syntax, component manifest, command, and cheatsheet examples replace the nonexistent ECR repository with an explicitly marked `YOUR_PUBLIC_ALIAS` placeholder requiring a published component repository/tag.

## Validation

Executed `atmos vendor pull` twice against the original ECR URI in an isolated Git fixture with empty Docker credential configuration; both returned `NAME_UNKNOWN`. Executed the same command against `oci://docker.io/library/nginx:alpine`; it succeeded and materialized the nginx binary. Evidence: `.context/skills-field-test/yaml-public-oci-results.json` and `yaml-public-oci-control-results.json`. The scripts with those names minus `-results` recreate the fixture and invocation. These pulls do not run the container or deploy resources.

## Follow-ups

Project-owned Terraform OCI images require the project's registry, published repository/tag, and credentials where private. No existing public Terraform component repository is implied by the placeholders.
