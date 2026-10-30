# Fix: Discover module-only Terraform configurations in SBOMs

**Date:** 2026-10-22

## Summary

Terraform SBOM generation includes initialized configurations that use modules without external providers.

## Context

Discovery previously started exclusively from `.terraform.lock.hcl`. Terraform does not create that file for module-only configurations, so their modules were silently omitted while coverage reported no initialized configurations.

## Changes

Discover configuration roots from either provider lock files or `.terraform/modules/modules.json`, deduplicate roots, and skip downloaded module directories. Configurations without provider locks do not claim a nonexistent lock-file property. Their module graph and checksums use the existing Terraform adapter. The regression fixture uses the Go test executable as its Terraform subprocess, so it runs without a POSIX shell or Windows skip.

## Validation

A regression runs a local Terraform command fixture for an initialized module-only root and verifies its module, coverage, and absence of false lock metadata. An explicit prerequisite verifies subprocess environment propagation and module JSON output. The test requires the module coverage record to exist before checking its status and detail. The full SBOM package passes locally; Windows execution remains for CI. Live tests initialized a local child module with Terraform, generated provenance SBOMs twice without a provider lock, and verified the child and its checksum appeared.

## Follow-ups

Module discovery still requires Terraform's initialized local module manifest and the existing `terraform modules -json` support.
