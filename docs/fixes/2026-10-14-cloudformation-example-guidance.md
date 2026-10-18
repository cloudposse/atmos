# Fix: CloudFormation example lifecycle and policy paths

**Date:** 2026-10-14

## Summary

Align the manual example with its custom test command and link to the CLI reference.

## Context

The example omitted validation and delete confirmation flags. The policy documentation could be
read as requiring a second component-directory prefix.

## Changes

Include stack validation and explicit delete auto-approval. Link to the CloudFormation CLI docs
and describe stack policy paths as relative to the component directory under the base path.

## Validation

Compared the commands with the checked-in custom test command and confirmed the reference document
exists. The final combined website build remains part of stack validation before publication.

The generated Docusaurus metadata confirms the CLI reference permalink is
`/cli/commands/aws/cloudformation/`.

## Follow-ups

Complete the final website build and published CI checks in this active stack.
