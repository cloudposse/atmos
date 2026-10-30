# Fix: Reconcile migration guidance with verified capabilities

**Date:** 2026-10-22

## Summary

Corrected migration and toolchain skill claims that contradicted executed CLI controls and current command documentation.

## Context

The prose audit found obsolete claims about mise dependency graphs and updates, Aqua package types, component source declarations, component naming, GCP credential isolation, and tool installation paths. Original claim text is retained in the field-test prose ledger.

## Changes

Mise migration now describes `dependencies.commands`, command environment mappings, and `toolchain update --dry-run`/`update`. Aqua migration distinguishes supported downloads from unsupported build/install types. Terragrunt migration allows stack component sources. Terramate and remote-state bridge guidance requires preserving actual backend/workspace mappings and logical component keys instead of treating `metadata.name` as a state-prefix guarantee or lookup alias. Azure/GCP guidance uses `spec:` for provider fields and acknowledges the ADC provider reads existing default ADC credentials. The toolchain path example uses owner/tool/version without an OS directory.

Corresponding website command/configuration and migration pages were reviewed: toolchain usage already lists the four supported download types, the update command documents its operation, and migration backend guidance already specifies explicit state keys. The GCP provider website now also explicitly documents service-account key ADC support.

## Validation

Existing actual CLI evidence under `.context/skills-field-test/`: `migration-make-control.json` and `root-migration-runs.json` (dependency ordering/deduplication), `yaml-inventory-controls.json` (command environment), `yaml-inventory-extra-results.json` (update dry-run and actual installation path), `yaml-core-runs/patterns-source-pull-repeat.json` (component source), `yaml-core-runs/patterns-stable-name-actual-v1.json` and `patterns-stable-name-actual-v2.json` (backend prefix), and `yaml-core-runs/migration-bridge-alias-valid-state-1.json` and `migration-bridge-alias-valid-state-2.json` (lookup naming). GCP schema and ADC controls are recorded in `auth-example-fixed-results.json` and `auth-website-controls-results.json`. Package type documentation reconciliation does not claim newly executed github_content/github_archive downloads.

Additional actual ADC control: `yaml-prose-auth-results.json` records two `auth exec` invocations using an owned generated service-account key and a localhost OAuth token endpoint. `yaml-prose-auth-http-requests.json` confirms signed JWT grants; each child process receives an existing ADC file. This disproves the old claim that no provider loads downloaded service-account keys without claiming a real GCP account exchange.

## Follow-ups

Prose coverage remains explicit about untested package download variants and real cloud credential exchanges. These documentation corrections do not introduce product behavior or turn partial runtime coverage into a full pass.
