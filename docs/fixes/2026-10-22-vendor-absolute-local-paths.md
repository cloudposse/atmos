# Fix: Preserve absolute vendor source and target paths

**Date:** 2026-10-22

## Summary

Local file URLs and absolute target paths now retain their root.

## Context

The broad skills audit found that local file URLs lost the leading slash from their absolute paths and absolute targets were joined under the manifest directory a second time. Component manifests already had correct cross-platform file URI logic.

## Changes

Share the component file-URI resolver with vendor manifests, retaining percent-decoding, Windows drive letters and UNC paths. Preserve absolute targets and resolve relative source paths into the caller variable. Correct the major/minor version template to use Sprig slice instead of passing a count to first.

## Validation

Fail-before tests reproduced both path errors. Focused vendor, target, and file URI tests pass. Fourteen live vendor/retry controls include two successful file URI, absolute target, relative source/target, and version-template runs; each verifies materialized file contents.

## Follow-ups

None.
