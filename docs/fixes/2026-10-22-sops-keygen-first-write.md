# Fix: Find SOPS recipients beside the encrypted file

**Date:** 2026-10-22

## Summary

After `secret keygen`, the first secret write can discover `.sops.yaml` in the same directory as its encrypted file.

## Context

With `spec.file: secrets.enc.yaml`, key generation created the private key and `.sops.yaml`, but `secret set` failed to find the creation rule. The unchanged baseline reproduced this twice; configuring the generated public recipient explicitly allowed the write. SOPS already searches the parent of its supplied file path, and Atmos supplied the directory, skipping the correct first search location.

## Changes

Pass the absolute encrypted-file path to SOPS configuration discovery. Existing upward search and explicit recipient precedence remain intact.

## Validation

A new keygen-to-first-write regression failed before the fix. It now generates keys, encrypts a sibling file without explicit recipients, checks that plaintext is absent, and decrypts the original value. The full SOPS provider suite also covers nested-file discovery, missing/malformed creation rules and explicit recipients.

## Follow-ups

None.
