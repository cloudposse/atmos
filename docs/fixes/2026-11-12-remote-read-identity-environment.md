# Fix: identity environments now reach `atmos git`, `atmos ansible playbook`, and stored plugin credentials

**Date:** 2026-11-12

## Summary

Three places that should have used an Atmos Auth identity did not. `atmos git` ignored
`git.repositories.<name>.auth.identity` and the `--identity` flag. `atmos ansible playbook --identity`
authenticated but never gave the identity's environment to `ansible-playbook`. A plugin credential of
kind `environment/v1` that was stored by an earlier process failed in a fresh process.

## Context

These surfaced while adding `identity:` to vendored sources, component sources, and remote stack
imports, which all depend on an identity's environment reaching the subprocess that does the work.

- `atmos git` built each Git subprocess environment from the process environment only, so a configured
  repository identity (for example a `github/sts`-linked identity) and `--identity` had no effect.
- The Ansible executor processed the stack with the identity but then built the playbook environment from
  the component environment alone. Helmfile and Packer already layered the identity's environment on top.
- After `atmos auth login`, a later process finds the stored credential still valid and skips
  authentication. The `environment/v1` materializer was only populated by authentication, so in that fresh
  process (`atmos auth exec`, `atmos auth shell`, `atmos git`) it held no environment and the command failed.

## Changes

- `atmos git` composes the Git subprocess environment from the process environment plus the effective
  identity. The global `--identity` flag (or `ATMOS_IDENTITY`) wins over the repository's configured
  identity. With neither, the process environment is used unchanged.
- `atmos ansible playbook` layers the identity's prepared environment over the component environment before
  starting `ansible-playbook`, the same way Helmfile and Packer do.
- The `environment/v1` materializer can now be restored from a credential read back from the credential
  store. It applies the same validation and secret masking as a fresh authentication, and refuses an
  expired or malformed credential.

## Validation

- Unit tests cover the identity environment reaching the `atmos git` subprocess, the Ansible playbook
  environment with an auth manager, without one, and when preparation fails, and restoring a stored
  `environment/v1` credential, including refusal of an expired one.
- The website build covers the documentation for these behaviors.

## Follow-ups

None.
