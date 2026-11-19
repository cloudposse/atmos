# Fix: Source authentication opt-outs and explicit workdir placement

**Date:** 2026-10-28

## Summary

Source commands and Terraform shell use normalized identity-disable values. Explicit source
working directories now have consistent placement with an edition boundary and migration guard.

## Context

Main and the reconciled PR rejected `--identity=false` for source pull, and ignored an explicit
working directory when workdir provisioning was enabled. The metadata spelling was absent from
the manifest schema.

## Changes

Source and shell parsing uses the shared identity selector; library boundaries normalize false
aliases, and shell preserves disabled authentication during nested evaluation. Source placement
uses `ResolveTarget` for provisioning, execution reconstruction, output and local-state lookup.
The 2026-10-28 behavior transition preserves generated paths for older edition pins. A nonempty
old workdir blocks an implicit move; Atmos never moves or discards Terraform state automatically.
The manifest schema accepts the metadata override.
The shared resolver validates explicit-target ownership before state or output lookup,
including when automatic output provisioning is disabled. Publication retains its
under-lock ownership recheck.

## Validation

Flag/library tests cover false aliases and retained named-identity parsing. Edition tests cover
before/on/after, unpinned and invalid pins, metadata/settings overrides, and byte-preserved old
state on migration refusal. Real source pull and root/nested override apply/plan/output pass twice
on Terraform 1.15.6 and OpenTofu 1.12.5. Full validation evidence is retained under
`.context/terraform-repair-20261028/`.
Same-stack and cross-stack owner collisions are rejected by state/output lookups
without changing the owner's files. Source/output suites, local backend tests and
repeated ownership concurrency race checks pass.

## Follow-ups

Existing projects adopting the new precedence must explicitly migrate an old workdir when present;
the migration contract is documented in `docs/prd/source-provisioner.md`.
