# Fix: Edit inherited version entries through track overrides

**Date:** 2026-10-22

## Summary

`version track set` can update an entry inherited from the base catalog or a parent track.

## Context

The command only recognized entries physically present under the selected track, so valid inherited entries failed with “version entry not found.”

## Changes

Resolve the effective track catalog before rejecting a missing local entry. For inherited entries, write only the requested fields as an override in the selected track. The base catalog and independent tracks retain their values. Descendant tracks inherit changed fields unless they override them.

## Validation

A regression updates a base entry through the dev track and verifies the production track and base remain unchanged. Unknown entries still fail. Live CLI checks repeat the edit and inspect the resulting YAML.

## Follow-ups

Removing an inherited entry requires separate semantics and is unchanged.
